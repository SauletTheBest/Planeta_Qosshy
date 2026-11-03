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

	switch order.Status {
	case models.OrderPending:
		return markPaid(tx, order.ID, now)
	case models.OrderCancelled:
		// Paid after the reservation expired and the stock went back on sale: take it again.
		// A nested Transaction is a SAVEPOINT: if the stock is gone, only this part is undone.
		err := tx.Transaction(func(sp *gorm.DB) error { return reserveItems(sp, order.Items) })
		if err == nil {
			return markPaid(tx, order.ID, now)
		}
		if !errors.Is(err, ErrOutOfStock) {
			return err
		}
		log.Printf("REFUND NEEDED: order %d was paid (payment %d) after its reservation expired and the item is sold out", order.ID, p.ID)
		return nil
	default:
		log.Printf("REFUND NEEDED: order %d is already %s, payment %d paid it again", order.ID, order.Status, p.ID)
		return nil
	}
}

func markPaid(tx *gorm.DB, orderID uint, now time.Time) error {
	return tx.Model(&models.Order{}).Where("id = ?", orderID).Updates(map[string]any{
		"status":     models.OrderPaid,
		"paid_at":    now,
		"expires_at": nil, // no reservation to release anymore
	}).Error
}
