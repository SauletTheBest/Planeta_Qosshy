package shop

import (
	"context"
	"log"
	"time"

	"planeta_qosshy/models"

	"gorm.io/gorm"
)

// CancelExpiredOrders cancels pending orders whose reservation ended (plus ExpiryGrace)
// and puts their items back on sale. Safe to run from several app instances at once:
// each order is cancelled by whichever conditional UPDATE comes first, the rest do nothing.
func CancelExpiredOrders(db *gorm.DB, now time.Time) (int, error) {
	var ids []uint
	if err := db.Model(&models.Order{}).
		Where("status = ? AND expires_at < ?", models.OrderPending, now.Add(-ExpiryGrace)).
		Order("id").Limit(100).
		Pluck("id", &ids).Error; err != nil {
		return 0, err
	}

	cancelled := 0
	for _, id := range ids {
		done := false
		err := db.Transaction(func(tx *gorm.DB) error {
			// WHERE status = pending: if a webhook paid the order a moment ago, this changes
			// nothing (Postgres re-checks the condition after waiting for the webhook's lock).
			res := tx.Model(&models.Order{}).
				Where("id = ? AND status = ?", id, models.OrderPending).
				Update("status", models.OrderCancelled)
			if res.Error != nil || res.RowsAffected == 0 {
				return res.Error
			}

			var items []models.OrderItem
			if err := tx.Where("order_id = ?", id).Find(&items).Error; err != nil {
				return err
			}
			if err := releaseItems(tx, items); err != nil {
				return err
			}
			done = true
			return tx.Model(&models.Payment{}).
				Where("order_id = ? AND status = ?", id, models.PaymentPending).
				Updates(map[string]any{"status": models.PaymentFailed, "failure_reason": "expired"}).Error
		})
		if err != nil {
			return cancelled, err
		}
		if done {
			cancelled++
		}
	}
	return cancelled, nil
}

// RunExpiryLoop cancels expired orders every ExpiryInterval until ctx is cancelled.
func RunExpiryLoop(ctx context.Context, db *gorm.DB) {
	ticker := time.NewTicker(ExpiryInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			n, err := CancelExpiredOrders(db, now)
			if err != nil {
				log.Println("order expiry:", err)
			} else if n > 0 {
				log.Printf("order expiry: cancelled %d unpaid order(s), stock returned", n)
			}
		}
	}
}
