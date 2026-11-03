package models

import (
	"time"
)

// Deprecated: the code no longer writes transactions; Payment replaced it in
// migration 000003. The table is kept only as a read-only archive of old purchases.
type Transaction struct {
	ID            uint      `gorm:"primaryKey"`
	Title         string    `gorm:"not null"`
	UserName      string    `gorm:"not null"`
	Price         float64   `gorm:"not null"`
	Quantity      int       `gorm:"default:1"`
	TotalAmount   float64   `gorm:"not null"`
	PaymentMethod string    `gorm:"not null"`
	Status        string    `gorm:"default:'pending'"`
	CreatedAt     time.Time `gorm:"autoCreateTime"`
}
