package data

import "time"

// Order is a simplified transactional record that we can query inefficiently.
type Order struct {
	ID              uint   `gorm:"primaryKey"`
	CustomerID      uint   `gorm:"index:idx_orders_customer_id"`
	CustomerName    string `gorm:"size:64;index"`
	Phone           string `gorm:"size:32;index:idx_orders_phone"`
	Status          string `gorm:"size:32;index"`
	ProductCategory string `gorm:"size:32;index"`
	Region          string `gorm:"size:32;index"`
	TotalAmount     float64
	DiscountCode    string     `gorm:"size:32"`
	Note            string     `gorm:"size:255"`
	CreatedAt       time.Time  `gorm:"index"`
	UpdatedAt       time.Time  `gorm:"index"`
	ShippedAt       *time.Time `gorm:"index"`
}
