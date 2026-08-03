package models

import (
	"time"
)

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
