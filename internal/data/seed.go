package data

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"gorm.io/gorm"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// SeedConfig controls how many orders are inserted for experiments.
type SeedConfig struct {
	Orders    int
	BatchSize int
}

// EnsureSchema applies the required database schema.
func EnsureSchema(db *gorm.DB) error {
	return db.AutoMigrate(&Order{})
}

// SeedDataset populates the database with deterministic synthetic data.
func SeedDataset(ctx context.Context, db *gorm.DB, cfg SeedConfig) error {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 1000
	}
	if cfg.Orders < CoveringCustomerTarget {
		cfg.Orders = CoveringCustomerTarget
	}
	return seedOrders(ctx, db, cfg)
}

func seedOrders(ctx context.Context, db *gorm.DB, cfg SeedConfig) error {
	var existing int64
	if err := db.WithContext(ctx).Model(&Order{}).Count(&existing).Error; err != nil {
		return err
	}
	if int(existing) >= cfg.Orders {
		return nil
	}

	toCreate := cfg.Orders - int(existing)
	batch := make([]Order, 0, cfg.BatchSize)
	now := time.Now()
	rnd := rand.New(rand.NewSource(42))
	start := int(existing)

	for i := 0; i < toCreate; i++ {
		order := buildSyntheticOrder(start+i, rnd, now)
		batch = append(batch, order)

		if len(batch) == cfg.BatchSize || i == toCreate-1 {
			if err := db.WithContext(ctx).Create(&batch).Error; err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	return nil
}

func buildSyntheticOrder(globalIdx int, rnd *rand.Rand, now time.Time) Order {
	var customerID uint
	if globalIdx < 1000 {
		customerID = coveringCustomerID
	} else {
		customerID = uint(rnd.Intn(50000) + 1)
	}

	created := now.Add(-time.Duration(rnd.Intn(365*24)) * time.Hour)
	var shipped *time.Time
	if rnd.Float64() > 0.3 {
		s := created.Add(time.Duration(rnd.Intn(72)) * time.Hour)
		shipped = &s
	}

	order := Order{
		CustomerID:      customerID,
		CustomerName:    customerName(customerID),
		Phone:           randomPhone(rnd),
		Status:          randomChoiceWeighted(statuses, rnd),
		ProductCategory: randomChoice(categories, rnd),
		Region:          randomChoice(regions, rnd),
		TotalAmount:     10 + rnd.Float64()*990,
		DiscountCode:    discountCode(rnd),
		Note:            randomChoice(loremSamples, rnd),
		CreatedAt:       created,
		UpdatedAt:       created,
		ShippedAt:       shipped,
	}
	if customerID != coveringCustomerID {
		order.Phone = randomPhone(rnd)
	}
	return order
}

func customerName(id uint) string {
	return fmt.Sprintf("Customer %06d", id)
}

func discountCode(rnd *rand.Rand) string {
	return fmt.Sprintf("CODE%02d", rnd.Intn(100))
}

var (
	regions      = []string{"north", "south", "east", "west"}
	statuses     = []string{"pending", "paid", "fulfilled", "cancelled"}
	categories   = []string{"fashion", "electronics", "books", "grocery", "home"}
	loremSamples = []string{
		"Need gift wrap and rush delivery please.",
		"Customer called to change shipping address.",
		"Large wholesale order awaiting approval.",
		"Repeat customer eligible for loyalty perks.",
		"Flagged for manual fraud review before shipment.",
	}
)

func randomChoice(items []string, rnd *rand.Rand) string {
	return items[rnd.Intn(len(items))]
}

func randomChoiceWeighted(items []string, rnd *rand.Rand) string {
	weights := map[string]int{
		"pending":   30,
		"paid":      40,
		"fulfilled": 20,
		"cancelled": 10,
	}
	total := 0
	for _, item := range items {
		total += weights[item]
	}
	n := rnd.Intn(total)
	for _, item := range items {
		n -= weights[item]
		if n < 0 {
			return item
		}
	}
	return items[0]
}

func randomPhone(rnd *rand.Rand) string {
	prefixes := []string{"138", "139", "137", "188", "199"}
	prefix := prefixes[rnd.Intn(len(prefixes))]
	return fmt.Sprintf("%s%08d", prefix, rnd.Intn(100000000))
}
