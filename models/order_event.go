package models

import "time"

// Actions recorded in an order's history.
const (
	EventPaid         = "paid"
	EventExpired      = "expired"
	EventShipped      = "shipped"
	EventDelivered    = "delivered"
	EventCancelled    = "cancelled"
	EventRefundNeeded = "refund_needed"
	EventRefunded     = "refunded"
	EventRefundFailed = "refund_failed"
)

// OrderEvent is one line of an order's history: what happened, who did it, when.
// Rows are only ever added, never changed — that is what makes it an audit log.
type OrderEvent struct {
	ID        uint `gorm:"primaryKey"`
	OrderID   uint `gorm:"not null"`
	Action    string
	ActorID   *uint // nil = the system (expiry job) or the bank (webhook)
	Note      string
	CreatedAt time.Time `gorm:"autoCreateTime"`

	Actor *User `gorm:"foreignKey:ActorID"`
}

// Label is the action in words for the admin page.
func (e OrderEvent) Label() string {
	switch e.Action {
	case EventPaid:
		return "Оплачен"
	case EventExpired:
		return "Отменён: время на оплату истекло"
	case EventShipped:
		return "Отправлен"
	case EventDelivered:
		return "Доставлен"
	case EventCancelled:
		return "Отменён"
	case EventRefundNeeded:
		return "Нужен возврат денег"
	case EventRefunded:
		return "Деньги возвращены"
	case EventRefundFailed:
		return "Возврат не прошёл"
	}
	return e.Action
}
