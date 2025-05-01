package models

import (
	"time"
)

type Transaction struct {
	ID            uint `gorm:"primaryKey"`
	Model         string
	UserName      string
	Price         float64
	Quantity      int
	TotalAmount   float64
	PaymentMethod string
	Status        string
	CreatedAt     time.Time `gorm:"autoCreateTime"`
}
