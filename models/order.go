package models

import (
	"time"

	"planeta_qosshy/money"
)

type OrderStatus string

const (
	OrderPending   OrderStatus = "pending" // created, waiting for payment
	OrderPaid      OrderStatus = "paid"
	OrderCancelled OrderStatus = "cancelled"
	OrderShipped   OrderStatus = "shipped"
	OrderDelivered OrderStatus = "delivered"
)

// Label is the status as shown to customers.
func (s OrderStatus) Label() string {
	switch s {
	case OrderPending:
		return "Ожидает оплаты"
	case OrderPaid:
		return "Оплачен"
	case OrderCancelled:
		return "Отменён"
	case OrderShipped:
		return "Отправлен"
	case OrderDelivered:
		return "Доставлен"
	}
	return string(s)
}

type Order struct {
	ID        uint        `gorm:"primaryKey"`
	UserID    uint        `gorm:"not null;index"`
	Status    OrderStatus `gorm:"not null"`
	TotalTiyn money.Tiyn  `gorm:"not null"`
	PaidAt    *time.Time  // nil until the order is paid
	ExpiresAt *time.Time  // pending orders only: the reserved stock is released after this
	CreatedAt time.Time   `gorm:"autoCreateTime"`
	UpdatedAt time.Time   `gorm:"autoUpdateTime"`

	Items    []OrderItem `gorm:"foreignKey:OrderID"`
	Payments []Payment   `gorm:"foreignKey:OrderID"` // load newest first to use LastPayment
}

// LastPayment is the newest attempt, if Payments were loaded newest first.
func (o Order) LastPayment() *Payment {
	if len(o.Payments) == 0 {
		return nil
	}
	return &o.Payments[0]
}

// SucceededPayment is the attempt that actually took the money, if any.
func (o Order) SucceededPayment() *Payment {
	for i := range o.Payments {
		if o.Payments[i].Status == PaymentSucceeded {
			return &o.Payments[i]
		}
	}
	return nil
}

// OrderItem is a snapshot of one purchased product. Title and price are copied at
// purchase time, so editing or deleting the product later doesn't rewrite history.
type OrderItem struct {
	ID            uint       `gorm:"primaryKey"`
	OrderID       uint       `gorm:"not null;index"`
	ClothesID     *uint      // nil if the product was deleted after the purchase
	Title         string     `gorm:"not null"`
	Size          string     // empty if no size was chosen
	UnitPriceTiyn money.Tiyn `gorm:"not null"`
	Quantity      int        `gorm:"not null"`

	Clothes *Clothes `gorm:"foreignKey:ClothesID"` // only for the picture; may be nil
}
