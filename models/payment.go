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
	PaymentRefunded  PaymentStatus = "refunded" // the bank gave the money back
)

// Label is the status as shown in the admin panel.
func (s PaymentStatus) Label() string {
	switch s {
	case PaymentPending:
		return "Ожидает"
	case PaymentSucceeded:
		return "Оплачен"
	case PaymentFailed:
		return "Не прошёл"
	case PaymentRefunded:
		return "Возвращён"
	}
	return string(s)
}

// Payment is one attempt to pay an order. An order can have several
// (a declined card, then a successful one), so it's a separate table.
type Payment struct {
	ID                uint          `gorm:"primaryKey"`
	OrderID           uint          `gorm:"not null;index"`
	AmountTiyn        money.Tiyn    `gorm:"not null"`
	Method            string        `gorm:"not null"` // "card"
	Status            PaymentStatus `gorm:"not null"`
	Provider          string        `gorm:"not null"` // "qosshypay"; "simulated" for old rows
	ProviderPaymentID *string       // the bank's ID; nil until the bank created the session
	PaymentURL        string        // the bank page for the buyer
	CardLast4         string
	FailureReason     string
	ReceiptURL        string
	RefundNeededAt    *time.Time // set when the shop owes this money back
	RefundedAt        *time.Time
	CreatedAt         time.Time `gorm:"autoCreateTime"`
	UpdatedAt         time.Time `gorm:"autoUpdateTime"`
}

// NeedsRefund: the bank took the money, but the shop has to give it back.
func (p Payment) NeedsRefund() bool {
	return p.Status == PaymentSucceeded && p.RefundNeededAt != nil
}

// FailureText explains a failed attempt to the buyer.
func (p Payment) FailureText() string {
	switch p.FailureReason {
	case "card_declined":
		return "Банк отклонил операцию"
	case "insufficient_funds":
		return "Недостаточно средств на карте"
	case "expired":
		return "Время на оплату истекло"
	case "provider_unavailable":
		return "Платёжный сервис временно недоступен"
	case "not_started":
		return "Оплата не была начата"
	case "cancelled":
		return "Заказ отменён магазином"
	}
	return p.FailureReason
}
