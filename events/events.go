// Package events is the CONTRACT between the shop, which writes to Kafka, and everyone
// who reads from it (the notifier today, maybe analytics tomorrow). Changing this file is
// changing an API: add new fields, never rename or remove existing ones — a reader that
// was built against the old version must still understand the message.
package events

import (
	"strconv"
	"time"
)

const (
	// TopicOrderEvents receives one message per thing that happens to an order.
	TopicOrderEvents = "shop.order-events"

	// Version of the OrderEvent layout; bump it (and keep old fields) on a big change.
	Version = 1
)

// Event types (the same words as the order history in the admin panel).
const (
	TypePaid         = "paid"
	TypeExpired      = "expired"
	TypeShipped      = "shipped"
	TypeDelivered    = "delivered"
	TypeCancelled    = "cancelled"
	TypeRefundNeeded = "refund_needed"
	TypeRefunded     = "refunded"
	TypeRefundFailed = "refund_failed"
)

// StatusCancelled is the order status (OrderEvent.Status) of a cancelled order.
const StatusCancelled = "cancelled"

// OrderEvent is one thing that happened to an order. It carries everything a reader
// needs — customer, items, total — so the reader never has to query the shop's database
// ("event-carried state transfer"): the services stay independent.
type OrderEvent struct {
	EventID    string    `json:"event_id"` // unique; readers use it to drop duplicates
	Version    int       `json:"version"`
	Type       string    `json:"type"`
	OrderID    uint      `json:"order_id"`
	Status     string    `json:"status"` // the order's status after this event
	TotalTiyn  int64     `json:"total_tiyn"`
	Customer   Customer  `json:"customer"`
	Items      []Item    `json:"items"`
	Note       string    `json:"note,omitempty"` // e.g. the reason of a cancellation
	OccurredAt time.Time `json:"occurred_at"`
}

type Customer struct {
	ID       uint   `json:"id"`
	Email    string `json:"email"`
	Username string `json:"username"`
}

type Item struct {
	Title         string `json:"title"`
	Size          string `json:"size,omitempty"`
	Quantity      int    `json:"quantity"`
	UnitPriceTiyn int64  `json:"unit_price_tiyn"`
}

// Key is the Kafka message key. Kafka puts messages with the same key into the same
// partition and keeps them in order there, so "paid" is always read before "shipped".
func (e OrderEvent) Key() string { return strconv.FormatUint(uint64(e.OrderID), 10) }
