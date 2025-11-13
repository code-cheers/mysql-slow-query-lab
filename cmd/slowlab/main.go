package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"
	"text/tabwriter"
	"time"
	"unicode/utf8"

	"mysql-slow-query-lab/internal/data"
	"mysql-slow-query-lab/internal/db"

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
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "类型\t子序号\t场景\t说明(截断)\t耗时\t行数\t状态")
	currentType := ""
	typeCounter := 0
	for _, res := range results {
		if res.Type != "" && res.Type != currentType {
			currentType = res.Type
			typeCounter = 0
			fmt.Fprintf(tw, "%s\t\t\t\t\t\t\n", strings.Repeat("═", utf8.RuneCountInString(currentType)+6))
			fmt.Fprintf(tw, "%s\t\t\t\t\t\t\n", currentType)
		}
		typeCounter++
		status := "OK"
		if res.Err != nil {
			status = "ERR: " + res.Err.Error()
		}
		desc := truncateText(res.Description, 40)
		fmt.Fprintf(tw, "\t%2d\t%-16s\t%-40s\t%12s\t%10d\t%s\n", typeCounter, res.Name, desc, res.Duration, res.RowCount, status)
		fmt.Fprintf(tw, "\t\t%s\t%s\t%s\t%s\t%s\n",
			strings.Repeat("─", 16),
			strings.Repeat("─", 40),
			strings.Repeat("─", 12),
			strings.Repeat("─", 10),
			strings.Repeat("─", len(status)))
	}
	tw.Flush()
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
