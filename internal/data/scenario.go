package data

import (
	"context"
	"fmt"
	"math/rand"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	coveringCustomerID     = 100
	CoveringCustomerTarget = 1000000
	DateRangeOrderTarget   = 2000
	phoneHotRowTarget      = 2000
	heavyHotNoteRuneLimit  = 70
	indexFuncDate          = "2024-01-01"
	dateTimeLayout         = "2006-01-02 15:04:05"
)

var (
	heavyHotNotePrefix = func() string {
		base := strings.Repeat("热点订单数据 ", 40)
		runes := []rune(base)
		if len(runes) > heavyHotNoteRuneLimit {
			runes = runes[:heavyHotNoteRuneLimit]
		}
		return string(runes)
	}()
	indexFuncRangeStart = mustParseDateTime(indexFuncDate + " 00:00:00")
	indexFuncRangeEnd   = indexFuncRangeStart.Add(24 * time.Hour)
	indexFuncRangeArgs  = []interface{}{
		indexFuncRangeStart.Format(dateTimeLayout),
		indexFuncRangeEnd.Format(dateTimeLayout),
	}
)

// Scenario describes a reproducible slow-query pattern.
type Scenario struct {
	Type        string
	Name        string
	Description string
	Query       string
	Args        []interface{}
	Setup       func(context.Context, *gorm.DB) error
}

// ScenarioResult captures timing and explain output for a scenario.
type ScenarioResult struct {
	Type        string
	Name        string
	Description string
	Duration    time.Duration
	RowCount    int64
	Explain     []string
	Err         error
}

// RunScenarios executes the built-in slow-query demonstrations.
func RunScenarios(ctx context.Context, db *gorm.DB) []ScenarioResult {
	scenarios := []Scenario{
		{
			Type:        "回表对比",
			Name:        "索引回表查询",
			Description: "使用 customer_id 二级索引定位后再取整行，需对每条记录回表。",
			Query:       "SELECT * FROM orders WHERE customer_id = ?",
			Args:        []interface{}{coveringCustomerID},
			Setup:       ensureHotCustomerOrders,
		},
		{
			Type:        "回表对比",
			Name:        "覆盖索引查询",
			Description: "同样条件只查 customer_id，可直接在二级索引中返回，避免回表。",
			Query:       "SELECT customer_id FROM orders WHERE customer_id = ?",
			Args:        []interface{}{coveringCustomerID},
			Setup:       ensureHotCustomerOrders,
		},
		{
			Type:        "索引字段做函数操作对比",
			Name:        "函数包裹索引列",
			Description: "DATE(created_at) 把时间字段包一层函数，索引失效。",
			Query:       "SELECT * FROM orders WHERE DATE(created_at) = ?",
			Args:        []interface{}{indexFuncDate},
			Setup:       ensureDateRangeOrders,
		},
		{
			Type:        "索引字段做函数操作对比",
			Name:        "范围查询命中索引",
			Description: "同样的日期条件改用范围过滤，优化器可使用 created_at 索引快速定位。",
			Query:       "SELECT * FROM orders WHERE created_at >= ? AND created_at < ?",
			Args:        indexFuncRangeArgs,
			Setup:       ensureDateRangeOrders,
		},
		{
			Type:        "类型匹配对比",
			Name:        "类型不匹配隐式转换",
			Description: "phone 列为字符串但使用数字常量比较，触发隐式转换并导致索引失效。",
			Query:       "SELECT * FROM orders WHERE phone = 13812345678",
			Setup:       ensurePhoneHotOrders,
		},
		{
			Type:        "类型匹配对比",
			Name:        "类型匹配命中索引",
			Description: "同样的 phone 条件改为字符串常量，索引可直接命中。",
			Query:       "SELECT * FROM orders WHERE phone = ?",
			Args:        []interface{}{PhoneHotValue},
			Setup:       ensurePhoneHotOrders,
		},
	}

	results := make([]ScenarioResult, 0, len(scenarios))
	for _, sc := range scenarios {
		res := ScenarioResult{Name: sc.Name, Description: sc.Description, Type: sc.Type}

		if sc.Setup != nil {
			if err := sc.Setup(ctx, db); err != nil {
				res.Err = fmt.Errorf("setup: %w", err)
				results = append(results, res)
				continue
			}
		}

		start := time.Now()
		rows, err := db.WithContext(ctx).Raw(sc.Query, sc.Args...).Rows()
		if err != nil {
			res.Err = err
			results = append(results, res)
			continue
		}

		var count int64
		for rows.Next() {
			count++
		}
		rows.Close()

		res.Duration = time.Since(start)
		res.RowCount = count

		explain, err := explainQuery(ctx, db, sc.Query, sc.Args...)
		if err == nil {
			res.Explain = explain
		} else {
			res.Explain = []string{fmt.Sprintf("failed to collect EXPLAIN: %v", err)}
		}

		results = append(results, res)
	}

	return results
}

func explainQuery(ctx context.Context, db *gorm.DB, query string, args ...interface{}) ([]string, error) {
	explainSQL := "EXPLAIN ANALYZE " + query
	lines, err := fetchExplain(ctx, db, explainSQL, args...)
	if err == nil {
		return lines, nil
	}
	return fetchExplain(ctx, db, "EXPLAIN "+query, args...)
}

func fetchExplain(ctx context.Context, db *gorm.DB, sql string, args ...interface{}) ([]string, error) {
	var rows []map[string]interface{}
	if err := db.WithContext(ctx).Raw(sql, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}

	lines := make([]string, 0, len(rows))
	for _, row := range rows {
		lineParts := make([]string, 0, len(row))
		for k, v := range row {
			lineParts = append(lineParts, fmt.Sprintf("%s=%v", k, v))
		}
		lines = append(lines, strings.Join(lineParts, " "))
	}
	return lines, nil
}

func ensureHotCustomerOrders(ctx context.Context, db *gorm.DB) error {
	var existing int64
	if err := db.WithContext(ctx).
		Model(&Order{}).
		Where("customer_id = ?", coveringCustomerID).
		Count(&existing).Error; err != nil {
		return err
	}

	if existing >= CoveringCustomerTarget {
		return nil
	}

	var template Order
	if err := db.WithContext(ctx).
		Where("customer_id = ?", coveringCustomerID).
		Order("id ASC").
		Take(&template).Error; err != nil {
		return fmt.Errorf("fetch template order: %w", err)
	}

	batch := make([]Order, 0, 1000)
	toInsert := CoveringCustomerTarget - existing
	for i := int64(0); i < toInsert; i++ {
		newOrder := template
		newOrder.ID = 0
		offset := time.Duration(existing+i) * time.Second
		newOrder.CreatedAt = template.CreatedAt.Add(offset)
		newOrder.UpdatedAt = newOrder.CreatedAt
		newOrder.Note = fmt.Sprintf("%s#%d", heavyHotNotePrefix, existing+i)
		if template.ShippedAt != nil {
			shipped := template.ShippedAt.Add(offset)
			newOrder.ShippedAt = &shipped
		} else {
			newOrder.ShippedAt = nil
		}
		batch = append(batch, newOrder)
		if len(batch) == cap(batch) || i == toInsert-1 {
			if err := db.WithContext(ctx).Create(&batch).Error; err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	return nil
}

func ensurePhoneHotOrders(ctx context.Context, db *gorm.DB) error {
	target := int64(phoneHotRowTarget)
	var existing int64
	if err := db.WithContext(ctx).
		Model(&Order{}).
		Where("phone = ?", PhoneHotValue).
		Count(&existing).Error; err != nil {
		return err
	}
	if existing >= target {
		return nil
	}

	batch := make([]Order, 0, 1000)
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := existing; i < target; i++ {
		created := time.Now().Add(-time.Duration(rnd.Intn(365*24)) * time.Hour)
		order := Order{
			CustomerID:      coveringCustomerID + 2000 + uint(i),
			CustomerName:    fmt.Sprintf("PhoneHot %06d", i),
			Phone:           PhoneHotValue,
			Status:          randomStatus(rnd),
			ProductCategory: "electronics",
			Region:          "east",
			TotalAmount:     199 + rnd.Float64()*50,
			DiscountCode:    "PHONEHOT",
			Note:            fmt.Sprintf("Phone hot sample #%d", i),
			CreatedAt:       created,
			UpdatedAt:       created,
		}
		batch = append(batch, order)
		if len(batch) == cap(batch) || i == target-1 {
			if err := db.WithContext(ctx).Create(&batch).Error; err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	return nil
}

func randomStatus(rnd *rand.Rand) string {
	statuses := []string{"pending", "paid", "fulfilled", "cancelled"}
	return statuses[rnd.Intn(len(statuses))]
}

func ensureDateRangeOrders(ctx context.Context, db *gorm.DB) error {
	target := int64(DateRangeOrderTarget)
	var existing int64
	if err := db.WithContext(ctx).
		Model(&Order{}).
		Where("created_at >= ? AND created_at < ?", indexFuncRangeStart, indexFuncRangeEnd).
		Count(&existing).Error; err != nil {
		return err
	}
	if existing >= target {
		return nil
	}

	toInsert := target - existing
	batch := make([]Order, 0, 2000)
	rnd := rand.New(rand.NewSource(time.Now().UnixNano()))

	for i := int64(0); i < toInsert; i++ {
		created := indexFuncRangeStart.Add(time.Duration(rnd.Intn(24*60*60)) * time.Second)
		var shipped *time.Time
		if rnd.Float64() > 0.4 {
			s := created.Add(time.Duration(rnd.Intn(48)+1) * time.Hour)
			shipped = &s
		}

		order := Order{
			CustomerID:      coveringCustomerID + 1000,
			CustomerName:    fmt.Sprintf("DateHot %06d", i),
			Status:          randomChoiceWeighted(statuses, rnd),
			ProductCategory: randomChoice(categories, rnd),
			Region:          randomChoice(regions, rnd),
			TotalAmount:     50 + rnd.Float64()*500,
			DiscountCode:    discountCode(rnd),
			Note:            fmt.Sprintf("日期热点订单 %s #%d", indexFuncDate, existing+i),
			CreatedAt:       created,
			UpdatedAt:       created,
			ShippedAt:       shipped,
		}
		batch = append(batch, order)

		if len(batch) == cap(batch) || i == toInsert-1 {
			if err := db.WithContext(ctx).Create(&batch).Error; err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	return nil
}

func mustParseDateTime(value string) time.Time {
	t, err := time.Parse(dateTimeLayout, value)
	if err != nil {
		panic(err)
	}
	return t
}
