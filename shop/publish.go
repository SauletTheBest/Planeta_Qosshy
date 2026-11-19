package shop

import (
	"fmt"

	"planeta_qosshy/events"
	"planeta_qosshy/models"
	"planeta_qosshy/outbox"

	"gorm.io/gorm"
)

// enqueueOrderEvent turns one line of the order history into a Kafka event and puts it
// into the outbox (the relay publishes it a moment later).
//
// The order is read inside the same transaction, so the event shows the order exactly as
// it is after this change (e.g. status "paid" in the "paid" event).
func enqueueOrderEvent(tx *gorm.DB, h models.OrderEvent) error {
	var o models.Order
	if err := tx.Preload("Items").Preload("User").First(&o, h.OrderID).Error; err != nil {
		return fmt.Errorf("event for order %d: %w", h.OrderID, err)
	}

	ev := events.OrderEvent{
		// Built from the history row's ID: unique, and the same if the message is
		// delivered twice — which is exactly what readers need to drop duplicates.
		EventID:    fmt.Sprintf("order-event-%d", h.ID),
		Version:    events.Version,
		Type:       h.Action,
		OrderID:    o.ID,
		Status:     string(o.Status),
		TotalTiyn:  int64(o.TotalTiyn),
		Note:       h.Note,
		OccurredAt: h.CreatedAt,
	}
	if o.User != nil {
		ev.Customer = events.Customer{ID: o.User.ID, Email: o.User.Email, Username: o.User.Username}
	}
	for _, it := range o.Items {
		ev.Items = append(ev.Items, events.Item{
			Title: it.Title, Size: it.Size, Quantity: it.Quantity, UnitPriceTiyn: int64(it.UnitPriceTiyn),
		})
	}
	return outbox.Enqueue(tx, events.TopicOrderEvents, ev.Key(), ev)
}
