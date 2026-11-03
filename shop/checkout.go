package shop

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"planeta_qosshy/models"
	"planeta_qosshy/payments"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrOutOfStock      = errors.New("out of stock")
	ErrNotPayable      = errors.New("the order can't be paid anymore")
	ErrPaymentStarting = errors.New("a payment for this order is being started right now")
)

// startingTimeout: a pending attempt without a bank page younger than this is assumed to be
// another request still talking to the bank; an older one is treated as abandoned.
const startingTimeout = 30 * time.Second

// PlaceOrder reserves one unit of the chosen size and creates a pending order that holds it
// until ExpiresAt. Nothing is paid yet: that happens at the bank (StartPayment).
func PlaceOrder(db *gorm.DB, userID uint, item models.Clothes, size string, now time.Time) (models.Order, error) {
	expires := now.Add(ReservationTTL)
	order := models.Order{
		UserID:    userID,
		Status:    models.OrderPending,
		TotalTiyn: item.PriceTiyn,
		ExpiresAt: &expires,
		Items: []models.OrderItem{{
			ClothesID:     &item.ID,
			Title:         item.Title,
			Size:          size,
			UnitPriceTiyn: item.PriceTiyn,
			Quantity:      1,
		}},
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		if err := reserveItems(tx, order.Items); err != nil {
			return err
		}
		return tx.Create(&order).Error // the order, then its items
	})
	return order, err
}

// StartPayment returns the bank page where the buyer pays the order. An attempt that is
// already open is reused, so pressing "pay" twice doesn't create two bank sessions.
func StartPayment(ctx context.Context, db *gorm.DB, orderID uint, customerEmail string, now time.Time) (string, error) {
	var order models.Order
	var payment models.Payment

	err := db.Transaction(func(tx *gorm.DB) error {
		// FOR UPDATE: two "pay" clicks for the same order queue up here instead of racing.
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, orderID).Error; err != nil {
			return err
		}
		if order.Status != models.OrderPending || order.ExpiresAt == nil || !now.Before(*order.ExpiresAt) {
			return ErrNotPayable
		}

		var open models.Payment
		err := tx.Where("order_id = ? AND status = ?", order.ID, models.PaymentPending).First(&open).Error
		switch {
		case err == nil && open.PaymentURL != "":
			payment = open
			return nil
		case err == nil && now.Sub(open.CreatedAt) < startingTimeout:
			return ErrPaymentStarting
		case err == nil:
			// An attempt that never got a bank page: the call failed or the process died.
			if err := tx.Model(&open).Updates(map[string]any{
				"status": models.PaymentFailed, "failure_reason": "not_started"}).Error; err != nil {
				return err
			}
		case !errors.Is(err, gorm.ErrRecordNotFound):
			return err
		}

		payment = models.Payment{
			OrderID:    order.ID,
			AmountTiyn: order.TotalTiyn,
			Method:     "card",
			Provider:   "qosshypay",
			Status:     models.PaymentPending,
		}
		return tx.Create(&payment).Error
	})
	if err != nil {
		return "", err
	}
	if payment.PaymentURL != "" {
		return payment.PaymentURL, nil
	}

	// The bank is called OUTSIDE the transaction: keeping the order row locked while
	// waiting for the network would block every other request that touches this order.
	resp, err := payments.CreatePayment(ctx, payments.CreateRequest{
		AmountTiyn:        int64(payment.AmountTiyn),
		Currency:          "KZT",
		Description:       fmt.Sprintf("Planeta Qosshy, заказ №%d", order.ID),
		MerchantReference: strconv.FormatUint(uint64(payment.ID), 10),
		CustomerEmail:     customerEmail,
		ReturnURL:         fmt.Sprintf("%s/orders/%d/result", PublicURL, order.ID),
		ExpiresAt:         *order.ExpiresAt,
	}, fmt.Sprintf("payment-%d", payment.ID))
	if err != nil {
		db.Model(&models.Payment{}).
			Where("id = ? AND status = ?", payment.ID, models.PaymentPending).
			Updates(map[string]any{"status": models.PaymentFailed, "failure_reason": "provider_unavailable"})
		return "", err
	}

	if err := db.Model(&models.Payment{}).Where("id = ?", payment.ID).Updates(map[string]any{
		"provider_payment_id": resp.PaymentID,
		"payment_url":         resp.PaymentURL,
	}).Error; err != nil {
		return "", err
	}
	return resp.PaymentURL, nil
}

// reserveItems takes the items out of stock: one atomic UPDATE per size, so two buyers
// can't both get the last unit. The DB trigger then updates clothes.stock.
func reserveItems(tx *gorm.DB, items []models.OrderItem) error {
	for _, it := range items {
		if it.ClothesID == nil {
			return ErrOutOfStock // the product was deleted
		}
		res := tx.Model(&models.ClothesVariant{}).
			Where("clothes_id = ? AND size = ? AND stock >= ?", *it.ClothesID, it.Size, it.Quantity).
			Update("stock", gorm.Expr("stock - ?", it.Quantity))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return ErrOutOfStock
		}
	}
	return nil
}

// releaseItems puts reserved items back on sale. A size the admin deleted meanwhile is skipped.
func releaseItems(tx *gorm.DB, items []models.OrderItem) error {
	for _, it := range items {
		if it.ClothesID == nil {
			continue
		}
		if err := tx.Model(&models.ClothesVariant{}).
			Where("clothes_id = ? AND size = ?", *it.ClothesID, it.Size).
			Update("stock", gorm.Expr("stock + ?", it.Quantity)).Error; err != nil {
			return err
		}
	}
	return nil
}
