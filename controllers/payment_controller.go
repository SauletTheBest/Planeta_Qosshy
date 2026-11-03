package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/payments"
	"planeta_qosshy/shop"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PaymentPage is the checkout page: what is being bought and a button to go to the bank.
func PaymentPage(c *gin.Context) {
	clothesID := c.Param("clothes_id")
	userID := c.GetUint("userID")

	var item models.Clothes
	if err := database.DB.Preload("Variants").First(&item, clothesID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Clothing item not found"})
		return
	}

	t, err := template.ParseFiles("templates/payment.html")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	t.Execute(c.Writer, gin.H{
		"userID":       userID,
		"clothesID":    clothesID,
		"amount":       item.PriceTiyn,
		"item":         item,
		"selectedSize": sizeToShow(item, c.Query("size")),
		"reserveMins":  int(shop.ReservationTTL.Minutes()),
	})
}

// Checkout reserves the chosen size, creates a pending order and sends the buyer to the bank.
// The order becomes paid only when the bank's signed webhook says so (PaymentWebhook).
func Checkout(c *gin.Context) {
	var request struct {
		Size string `form:"size"`
	}
	if err := c.ShouldBind(&request); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Invalid request parameters"})
		return
	}

	var item models.Clothes
	if err := database.DB.Preload("Variants").First(&item, c.Param("clothes_id")).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Товар не найден"})
		return
	}
	size, ok := sizeToBuy(item, request.Size)
	if !ok {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Выберите размер"})
		return
	}

	order, err := shop.PlaceOrder(database.DB, c.GetUint("userID"), item, size, time.Now())
	if errors.Is(err, shop.ErrOutOfStock) {
		c.HTML(http.StatusConflict, "error.html", gin.H{"error": "К сожалению, этот размер закончился"})
		return
	}
	if err != nil {
		log.Println("checkout:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось оформить заказ, попробуйте ещё раз"})
		return
	}

	redirectToBank(c, order.ID)
}

// RetryPayment opens a new payment attempt for a pending order (after a declined card, say).
func RetryPayment(c *gin.Context) {
	var order models.Order
	if err := database.DB.Where("id = ? AND user_id = ?", c.Param("id"), c.GetUint("userID")).First(&order).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Заказ не найден"})
		return
	}
	redirectToBank(c, order.ID)
}

func redirectToBank(c *gin.Context, orderID uint) {
	var user models.User
	database.DB.First(&user, c.GetUint("userID"))

	url, err := shop.StartPayment(c.Request.Context(), database.DB, orderID, user.Email, time.Now())
	if err != nil {
		log.Printf("order %d: start payment: %v", orderID, err)
		// The order and its reservation exist anyway: the result page explains what
		// happened and offers to try again.
		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/orders/%d/result", orderID))
		return
	}
	c.Redirect(http.StatusSeeOther, url)
}

// OrderResult is where the bank sends the buyer back. The page shows what the DATABASE
// says (filled in by the webhook), never what the URL claims: anyone can type "?paid=1".
func OrderResult(c *gin.Context) {
	var order models.Order
	err := database.DB.
		Preload("Items.Clothes").
		Preload("Payments", func(db *gorm.DB) *gorm.DB { return db.Order("id DESC") }).
		Where("id = ? AND user_id = ?", c.Param("id"), c.GetUint("userID")).
		First(&order).Error
	if err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Заказ не найден"})
		return
	}

	last := order.LastPayment()
	c.HTML(http.StatusOK, "order_result.html", gin.H{
		"order":   order,
		"payment": last,
		"paid":    order.SucceededPayment(),
		// While the bank's answer is on its way, the page reloads itself every few seconds.
		"waiting": order.Status == models.OrderPending && last != nil && last.Status == models.PaymentPending,
	})
}

// PaymentWebhook receives the bank's events. It has no session: it is authenticated by the
// HMAC signature alone, which only the bank can produce.
func PaymentWebhook(c *gin.Context) {
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "body too large"})
		return
	}

	err = payments.VerifySignature(payments.WebhookSecret(), c.GetHeader(payments.SignatureHeader), body, time.Now(), payments.DefaultTolerance)
	if err != nil {
		log.Println("webhook rejected:", err)
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid signature"})
		return
	}

	// Only after the signature check is the body trusted enough to be parsed.
	var ev payments.Event
	if err := json.Unmarshal(body, &ev); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON"})
		return
	}

	err = shop.ApplyPaymentEvent(database.DB, ev, time.Now())
	switch {
	case errors.Is(err, shop.ErrUnknownPayment), errors.Is(err, shop.ErrEventMismatch):
		log.Printf("webhook %s for %s rejected: %v", ev.Type, ev.PaymentID, err)
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
	case err != nil:
		log.Printf("webhook %s for %s: %v", ev.Type, ev.PaymentID, err)
		// 5xx tells the bank to retry later; the event is applied once it gets through.
		c.JSON(http.StatusInternalServerError, gin.H{"error": "temporary error"})
	default:
		c.JSON(http.StatusOK, gin.H{"ok": true})
	}
}

// sizeToBuy decides which variant a purchase is for: the requested size if the product
// has it, or the only size when there is exactly one. Whether it's still in stock is
// checked later by the atomic UPDATE, not here (it could change between the two).
func sizeToBuy(item models.Clothes, requested string) (string, bool) {
	requested = strings.TrimSpace(requested)
	if requested != "" {
		_, ok := item.Variant(requested)
		return requested, ok
	}
	if len(item.Variants) == 1 {
		return item.Variants[0].Size, true
	}
	return "", false
}

// sizeToShow picks the size for the checkout page: the requested one if it's in stock,
// otherwise the first size that is. The URL can be edited, so it's never trusted as is.
func sizeToShow(item models.Clothes, requested string) string {
	if v, ok := item.Variant(strings.TrimSpace(requested)); ok && v.Stock > 0 {
		return v.Size
	}
	for _, v := range item.SortedVariants() {
		if v.Stock > 0 {
			return v.Size
		}
	}
	return ""
}
