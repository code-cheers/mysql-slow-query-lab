package main

import (
	"context"
	"flag"
	"log"
	"os"
	"time"
	"unicode/utf8"

	"mysql-slow-query-lab/internal/data"
	"mysql-slow-query-lab/internal/db"

	"github.com/olekukonko/tablewriter"
	"github.com/olekukonko/tablewriter/renderer"
	"github.com/olekukonko/tablewriter/tw"
	"gorm.io/gorm"
)

func main() {
	var (
		orderCount    = flag.Int("orders", 1000000, "target number of orders to store")
		batchSize     = flag.Int("batch", 1000, "batch size for bulk inserts")
		skipSeed      = flag.Bool("skip-seed", false, "skip inserting synthetic data")
		skipScenarios = flag.Bool("skip-scenarios", false, "skip running slow query scenarios")
		showExplain   = flag.Bool("explain", true, "print EXPLAIN output for each scenario")
	)
	flag.Parse()

	if *orderCount < data.CoveringCustomerTarget {
		log.Printf("orders flag %d 小于热点查询所需的 %d，自动提升。", *orderCount, data.CoveringCustomerTarget)
		*orderCount = data.CoveringCustomerTarget
	}

	cfg := db.FromEnv()
	gdb, err := db.Open(cfg)
	if err != nil {
		log.Fatalf("failed to connect to MySQL: %v", err)
	}

	if err := data.EnsureSchema(gdb); err != nil {
		log.Fatalf("failed to migrate schema: %v", err)
	}

	ctx := context.Background()

	if !*skipSeed {
		start := time.Now()
		seedCfg := data.SeedConfig{
			Orders:    *orderCount,
			BatchSize: *batchSize,
		}
		if err := data.SeedDataset(ctx, gdb, seedCfg); err != nil {
			log.Fatalf("failed to seed dataset: %v", err)
		}
		log.Printf("dataset ready (orders target=%d) in %s", *orderCount, time.Since(start))
	} else {
		log.Printf("skip-seed enabled; reusing existing data")
	}

	if err := logDatasetStats(ctx, gdb); err != nil {
		log.Printf("failed to collect dataset stats: %v", err)
	}

	if *skipScenarios {
		log.Println("skip-scenarios enabled; exiting")
		return
	}

	results := data.RunScenarios(ctx, gdb)

	if *showExplain {
		for _, res := range results {
			if res.Err != nil {
				log.Printf("[scenario: %s] skipped explain due to error: %v", res.Name, res.Err)
				continue
			}
			log.Printf("[scenario: %s] %s", res.Name, res.Description)
			for _, line := range res.Explain {
				log.Printf("  %s", line)
			}
		}
	}

	printResultsTable(results)
}

func logDatasetStats(ctx context.Context, gdb *gorm.DB) error {
	var orders int64
	if err := gdb.WithContext(ctx).Model(&data.Order{}).Count(&orders).Error; err != nil {
		return err
	}
	minExpected := int64(data.CoveringCustomerTarget + data.DateRangeOrderTarget)
	log.Printf("当前数据量：orders=%d (最低预期≈%d，其中热点客户=%d，日期区间=%d)", orders, minExpected, data.CoveringCustomerTarget, data.DateRangeOrderTarget)
	return nil
}

func printResultsTable(results []data.ScenarioResult) {
	table := tablewriter.NewTable(os.Stdout,
		tablewriter.WithRenderer(renderer.NewBlueprint(tw.Rendition{
			Settings: tw.Settings{Separators: tw.Separators{BetweenRows: tw.On}},
		})),
		tablewriter.WithConfig(tablewriter.Config{
			Header: tw.CellConfig{Alignment: tw.CellAlignment{Global: tw.AlignCenter}},
			Row: tw.CellConfig{
				Merging:   tw.CellMerging{Mode: tw.MergeHierarchical},
				Alignment: tw.CellAlignment{Global: tw.AlignLeft},
			},
		}),
	)
	table.Header([]string{"类型", "子序号", "场景", "说明(截断)", "耗时", "行数", "状态"})
	currentType := ""
	typeCounter := 0
	for _, res := range results {
		if res.Type != "" && res.Type != currentType {
			currentType = res.Type
			typeCounter = 0
		}
		typeCounter++
		status := "OK"
		if res.Err != nil {
			status = "ERR: " + res.Err.Error()
		}
		desc := truncateText(res.Description, 40)
		err := table.Append([]any{res.Type, typeCounter, res.Name, desc, res.Duration, res.RowCount, status})
		if err != nil {
			log.Fatal(err)
		}
	}
	err := table.Render()
	if err != nil {
		log.Fatal(err)
	}
}

func truncateText(s string, limit int) string {
	if utf8.RuneCountInString(s) <= limit {
		return s
	}
	runes := []rune(s)
	if limit > len(runes) {
		limit = len(runes)
	}
	return string(runes[:limit]) + "…"
}
