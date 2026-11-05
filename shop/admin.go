package shop

import (
	"context"
	"errors"
	"fmt"
	"time"

	"planeta_qosshy/models"
	"planeta_qosshy/payments"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrBadTransition   = errors.New("this status change is not allowed")
	ErrNothingToRefund = errors.New("this payment does not need a refund")
)

// nextStatuses lists the only status changes an admin can make by hand.
// Everything else is done by the system: pending → paid by the bank's webhook,
// pending → cancelled by the expiry job. The admin pages draw their buttons from
// this same table, so a forbidden button can't even be shown.
var nextStatuses = map[models.OrderStatus][]models.OrderStatus{
	models.OrderPending: {models.OrderCancelled},
	models.OrderPaid:    {models.OrderShipped, models.OrderCancelled},
	models.OrderShipped: {models.OrderDelivered},
	// delivered and cancelled are final: nothing comes after them
}

// NextStatuses returns where an order in status `from` may be moved by an admin.
func NextStatuses(from models.OrderStatus) []models.OrderStatus {
	return nextStatuses[from]
}

// CanChange reports whether an admin may move an order from one status to another.
func CanChange(from, to models.OrderStatus) bool {
	for _, s := range nextStatuses[from] {
		if s == to {
			return true
		}
	}
	return false
}

// addEvent writes one line of the order's history inside the caller's transaction,
// so the history can never disagree with what really happened.
func addEvent(tx *gorm.DB, orderID uint, action string, actorID *uint, note string) error {
	return tx.Create(&models.OrderEvent{OrderID: orderID, Action: action, ActorID: actorID, Note: note}).Error
}

// lockOrder reads an order with SELECT ... FOR UPDATE: two admins pressing buttons on
// the same order at the same moment are served one after the other.
func lockOrder(tx *gorm.DB, orderID uint) (models.Order, error) {
	var order models.Order
	err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, orderID).Error
	return order, err
}

// ChangeStatus moves a paid order along: paid → shipped → delivered.
// Cancelling has its own function, because it also returns stock and money.
func ChangeStatus(db *gorm.DB, orderID uint, to models.OrderStatus, actorID uint) error {
	if to == models.OrderCancelled {
		return ErrBadTransition
	}
	return db.Transaction(func(tx *gorm.DB) error {
		order, err := lockOrder(tx, orderID)
		if err != nil {
			return err
		}
		if !CanChange(order.Status, to) {
			return ErrBadTransition
		}
		if err := tx.Model(&order).Update("status", to).Error; err != nil {
			return err
		}
		return addEvent(tx, order.ID, string(to), &actorID, "")
	})
}

// CancelOrder cancels a pending or paid order and puts its items back on sale.
// If the order was already paid, its payment is marked "refund needed" and its ID is
// returned: the caller then calls RefundPayment. The bank is NOT called here, because
// this runs inside a transaction and a slow network call would keep the order locked.
func CancelOrder(db *gorm.DB, orderID, actorID uint, reason string, now time.Time) (refundPaymentID uint, err error) {
	err = db.Transaction(func(tx *gorm.DB) error {
		order, err := lockOrder(tx, orderID)
		if err != nil {
			return err
		}
		if !CanChange(order.Status, models.OrderCancelled) {
			return ErrBadTransition
		}
		wasPaid := order.Status == models.OrderPaid

		if err := tx.Model(&order).Updates(map[string]any{
			"status": models.OrderCancelled, "expires_at": nil}).Error; err != nil {
			return err
		}

		var items []models.OrderItem
		if err := tx.Where("order_id = ?", order.ID).Find(&items).Error; err != nil {
			return err
		}
		if err := releaseItems(tx, items); err != nil {
			return err
		}

		// An open bank page for a pending order: payments made there from now on
		// will be refunded (see applySuccess), the order won't come back to life.
		if err := tx.Model(&models.Payment{}).
			Where("order_id = ? AND status = ?", order.ID, models.PaymentPending).
			Updates(map[string]any{"status": models.PaymentFailed, "failure_reason": "cancelled"}).Error; err != nil {
			return err
		}

		if err := addEvent(tx, order.ID, models.EventCancelled, &actorID, reason); err != nil {
			return err
		}

		if !wasPaid {
			return nil
		}
		var paid models.Payment
		if err := tx.Where("order_id = ? AND status = ?", order.ID, models.PaymentSucceeded).
			First(&paid).Error; err != nil {
			return fmt.Errorf("paid order %d has no successful payment: %w", order.ID, err)
		}
		refundPaymentID = paid.ID
		return markRefundNeeded(tx, paid, &actorID, "заказ отменён администратором", now)
	})
	return refundPaymentID, err
}

// markRefundNeeded records that the shop owes this payment's money back.
func markRefundNeeded(tx *gorm.DB, p models.Payment, actorID *uint, why string, now time.Time) error {
	if err := tx.Model(&models.Payment{}).Where("id = ?", p.ID).
		Update("refund_needed_at", now).Error; err != nil {
		return err
	}
	return addEvent(tx, p.OrderID, models.EventRefundNeeded, actorID,
		fmt.Sprintf("платёж №%d на %s: %s", p.ID, p.AmountTiyn, why))
}

// RefundPayment gives one payment's money back through the bank. It runs right after
// an admin cancels a paid order, and again from the refunds list if the bank was
// unreachable the first time. Repeating it is safe.
func RefundPayment(ctx context.Context, db *gorm.DB, paymentID uint, actorID *uint, now time.Time) error {
	var p models.Payment
	if err := db.First(&p, paymentID).Error; err != nil {
		return err
	}
	if p.Status == models.PaymentRefunded {
		return nil // already done (by an earlier click or by the bank's webhook)
	}
	if !p.NeedsRefund() || p.ProviderPaymentID == nil {
		return ErrNothingToRefund
	}

	// Outside any transaction, the same rule as in StartPayment.
	if _, err := payments.Refund(ctx, *p.ProviderPaymentID); err != nil {
		// Remember the failure in the order history; the payment stays in the refunds list.
		_ = addEvent(db, p.OrderID, models.EventRefundFailed, actorID, err.Error())
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error { return markRefunded(tx, p, actorID, now) })
}

// markRefunded is used by both the admin path and the bank's "payment.refunded"
// webhook, whichever comes first. The conditional UPDATE makes the second one a no-op.
func markRefunded(tx *gorm.DB, p models.Payment, actorID *uint, now time.Time) error {
	res := tx.Model(&models.Payment{}).
		Where("id = ? AND status = ?", p.ID, models.PaymentSucceeded).
		Updates(map[string]any{"status": models.PaymentRefunded, "refunded_at": now})
	if res.Error != nil || res.RowsAffected == 0 {
		return res.Error
	}
	return addEvent(tx, p.OrderID, models.EventRefunded, actorID,
		fmt.Sprintf("платёж №%d, %s", p.ID, p.AmountTiyn))
}

// cancelledByAdmin tells an order cancelled by an admin from one cancelled by the
// expiry job: only the second may be revived by a late payment.
func cancelledByAdmin(tx *gorm.DB, orderID uint) (bool, error) {
	var n int64
	err := tx.Model(&models.OrderEvent{}).
		Where("order_id = ? AND action = ?", orderID, models.EventCancelled).Count(&n).Error
	return n > 0, err
}
