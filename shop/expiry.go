package shop

import (
	"context"
	"log"
	"time"

	"planeta_qosshy/models"

	"gorm.io/gorm"
)

// expiryRunTimeout: one pass of the job may take at most this long. Without a limit a
// database that hangs (paused, overloaded, network gone) would hang the job forever.
const expiryRunTimeout = 30 * time.Second

// CancelExpiredOrders cancels pending orders whose reservation ended (plus ExpiryGrace)
// and puts their items back on sale. Safe to run from several app instances at once:
// each order is cancelled by whichever conditional UPDATE comes first, the rest do nothing.
//
// Every query runs with ctx: when ctx is cancelled (shutdown, timeout) the query in
// progress is aborted by the driver instead of waiting for the database to answer.
func CancelExpiredOrders(ctx context.Context, db *gorm.DB, now time.Time) (int, error) {
	db = db.WithContext(ctx)
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
			if err := tx.Model(&models.Payment{}).
				Where("order_id = ? AND status = ?", id, models.PaymentPending).
				Updates(map[string]any{"status": models.PaymentFailed, "failure_reason": "expired"}).Error; err != nil {
				return err
			}
			return addEvent(tx, id, models.EventExpired, nil, "товар вернулся в продажу")
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
			runCtx, cancel := context.WithTimeout(ctx, expiryRunTimeout)
			n, err := CancelExpiredOrders(runCtx, db, now)
			cancel()
			if ctx.Err() != nil {
				return // shutting down: the aborted query is expected, not an error to report
			}
			if err != nil {
				log.Println("order expiry:", err)
			} else if n > 0 {
				log.Printf("order expiry: cancelled %d unpaid order(s), stock returned", n)
			}
		}
	}
}
