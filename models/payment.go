package models

import (
	"time"

	"planeta_qosshy/money"
)

type PaymentStatus string

const (
	PaymentPending   PaymentStatus = "pending"
	PaymentSucceeded PaymentStatus = "succeeded"
	PaymentFailed    PaymentStatus = "failed"
)

// Payment is one attempt to pay an order. An order can have several
// (a failed card, then a successful one), so it's a separate table.
type Payment struct {
	ID         uint          `gorm:"primaryKey"`
	OrderID    uint          `gorm:"not null;index"`
	AmountTiyn money.Tiyn    `gorm:"not null"`
	Method     string        `gorm:"not null"` // "card"
	Status     PaymentStatus `gorm:"not null"`
	CreatedAt  time.Time     `gorm:"autoCreateTime"`
	UpdatedAt  time.Time     `gorm:"autoUpdateTime"`
}
