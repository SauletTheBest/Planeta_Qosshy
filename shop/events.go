package shop

import (
	"errors"
	"fmt"
	"log"
	"strconv"
	"time"

	"planeta_qosshy/models"
	"planeta_qosshy/money"
	"planeta_qosshy/payments"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrUnknownPayment = errors.New("unknown payment")
	ErrEventMismatch  = errors.New("event does not match the payment")
)

// ApplyPaymentEvent records what the bank reported. It is idempotent: the bank retries
// webhooks, so the same event can arrive several times and must change things only once.
func ApplyPaymentEvent(db *gorm.DB, ev payments.Event, now time.Time) error {
	return db.Transaction(func(tx *gorm.DB) error {
		var p models.Payment
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("provider_payment_id = ?", ev.PaymentID).First(&p).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrUnknownPayment
		}
		if err != nil {
			return err
		}

		// Even a correctly signed event must describe THIS payment for THIS amount.
		if strconv.FormatUint(uint64(p.ID), 10) != ev.MerchantReference ||
			p.AmountTiyn != money.Tiyn(ev.AmountTiyn) || ev.Currency != "KZT" {
			return ErrEventMismatch
		}

		switch ev.Type {
		case payments.EventSucceeded:
			return applySuccess(tx, p, ev, now)
		case payments.EventFailed:
			if p.Status != models.PaymentPending {
				return nil // already final; a late "failed" must not undo a payment
			}
			return tx.Model(&p).Updates(map[string]any{
				"status": models.PaymentFailed, "failure_reason": ev.FailureReason, "card_last4": ev.CardLast4}).Error
		case payments.EventRefunded:
			// Usually we asked for it ourselves (RefundPayment) and already marked it;
			// then nothing changes. If the refund was made at the bank directly, record it.
			if p.Status == models.PaymentRefunded {
				return nil
			}
			if p.Status != models.PaymentSucceeded {
				return ErrEventMismatch // money that was never taken can't be given back
			}
			return markRefunded(tx, p, nil, now)
		}
		return fmt.Errorf("unknown event type %q", ev.Type)
	})
}

func applySuccess(tx *gorm.DB, p models.Payment, ev payments.Event, now time.Time) error {
	if p.Status == models.PaymentSucceeded {
		return nil // the same webhook delivered again
	}
	// The bank took the money, so "succeeded" wins even over an attempt we had given up on
	// (marked failed when the reservation expired). Ignoring it would lose the buyer's money.
	if err := tx.Model(&p).Updates(map[string]any{
		"status":         models.PaymentSucceeded,
		"failure_reason": "",
		"card_last4":     ev.CardLast4,
		"receipt_url":    ev.ReceiptURL,
	}).Error; err != nil {
		return err
	}

	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("Items").First(&order, p.OrderID).Error; err != nil {
		return err
	}

	// Before: these cases were only a "REFUND NEEDED" line in the log. Now the payment is
	// marked, and the admin panel shows it in the refunds list until the money is back.
	refundNeeded := func(why string) error {
		log.Printf("REFUND NEEDED: order %d, payment %d: %s", order.ID, p.ID, why)
		return markRefundNeeded(tx, p, nil, why, now)
	}

	switch order.Status {
	case models.OrderPending:
		return markPaid(tx, order.ID, ev.CardLast4, now)
	case models.OrderCancelled:
		byAdmin, err := cancelledByAdmin(tx, order.ID)
		if err != nil {
			return err
		}
		if byAdmin {
			// An admin decided to cancel; a payment from a bank page that was still open
			// must not bring the order back.
			return refundNeeded("оплачен после отмены администратором")
		}
		// Paid after the reservation expired and the stock went back on sale: take it again.
		// A nested Transaction is a SAVEPOINT: if the stock is gone, only this part is undone.
		err = tx.Transaction(func(sp *gorm.DB) error { return reserveItems(sp, order.Items) })
		if err == nil {
			return markPaid(tx, order.ID, ev.CardLast4, now)
		}
		if !errors.Is(err, ErrOutOfStock) {
			return err
		}
		return refundNeeded("оплачен после окончания резерва, товар уже распродан")
	default:
		return refundNeeded(fmt.Sprintf("заказ уже в статусе «%s», это повторная оплата", order.Status.Label()))
	}
}

func markPaid(tx *gorm.DB, orderID uint, cardLast4 string, now time.Time) error {
	if err := tx.Model(&models.Order{}).Where("id = ?", orderID).Updates(map[string]any{
		"status":     models.OrderPaid,
		"paid_at":    now,
		"expires_at": nil, // no reservation to release anymore
	}).Error; err != nil {
		return err
	}
	return addEvent(tx, orderID, models.EventPaid, nil, "карта •••• "+cardLast4)
}
