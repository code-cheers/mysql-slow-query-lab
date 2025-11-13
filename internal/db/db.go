package db

import (
	"fmt"
	"os"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Config captures the connection parameters for a MySQL instance.
type Config struct {
	User     string
	Password string
	Host     string
	Port     string
	Database string
	Params   string
}

// FromEnv populates a Config using sensible defaults that can be overridden via environment variables.
func FromEnv() Config {
	cfg := Config{
		User:     getEnv("MYSQL_USER", "slowuser"),
		Password: getEnv("MYSQL_PASSWORD", "slowpass"),
		Host:     getEnv("MYSQL_HOST", "127.0.0.1"),
		Port:     getEnv("MYSQL_PORT", "3307"),
		Database: getEnv("MYSQL_DATABASE", "slowlab"),
		Params:   getEnv("MYSQL_PARAMS", "charset=utf8mb4&parseTime=True&loc=Local"),
	}
	return cfg
}

// Open returns a gorm DB using the provided configuration.
func Open(cfg Config) (*gorm.DB, error) {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?%s",
		cfg.User,
		cfg.Password,
		cfg.Host,
		cfg.Port,
		cfg.Database,
		cfg.Params,
	)

	gormCfg := &gorm.Config{
		Logger: logger.Default.LogMode(logger.Warn),
	}

	gdb, err := gorm.Open(mysql.Open(dsn), gormCfg)
	if err != nil {
		return nil, err
	}

	sqlDB, err := gdb.DB()
	if err != nil {
		return nil, err
	}

	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetMaxOpenConns(25)
	sqlDB.SetMaxIdleConns(5)

	return gdb, nil
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
