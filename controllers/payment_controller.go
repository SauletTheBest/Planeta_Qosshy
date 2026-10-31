package controllers

import (
	"errors"
	"html/template"
	"log"
	"net/http"
	"strings"
	"time"

	"planeta_qosshy/database"
	"planeta_qosshy/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func PaymentPage(c *gin.Context) {
	clothesID := c.Param("clothes_id")
	userID := c.GetUint("userID")

	var item models.Clothes
	if err := database.DB.First(&item, clothesID).Error; err != nil {
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
		"selectedSize": pickSize(item, c.Query("size")),
	})
}

var errOutOfStock = errors.New("out of stock")

func ProcessPayment(c *gin.Context) {
	userID := c.GetUint("userID")
	// Only the item and size are read from the form. Card fields are not bound at all, and
	// the price always comes from the DB: a hidden "amount" input can be edited in the browser.
	var request struct {
		ClothesID uint   `form:"clothes_id"`
		Size      string `form:"size"`
	}

	if err := c.ShouldBind(&request); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Invalid request parameters"})
		return
	}

	var item models.Clothes
	if err := database.DB.First(&item, request.ClothesID).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Товар не найден"})
		return
	}

	// One DB transaction: stock, order, its items and the payment are saved together or not at all.
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// Check and decrement in ONE statement, so two buyers can't both take the last item.
		// Inside SET, "stock" is the value before the update, so in_stock = (old stock - 1 > 0).
		res := tx.Model(&models.Clothes{}).
			Where("id = ? AND stock > 0", item.ID).
			Updates(map[string]interface{}{
				"stock":    gorm.Expr("stock - 1"),
				"in_stock": gorm.Expr("stock - 1 > 0"),
			})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return errOutOfStock // returning an error rolls the whole transaction back
		}

		now := time.Now()
		order := models.Order{
			UserID: userID,
			// The payment is still simulated, so the order is paid right away.
			// With a real payment service it will start as OrderPending.
			Status:    models.OrderPaid,
			TotalTiyn: item.PriceTiyn,
			PaidAt:    &now,
			Items: []models.OrderItem{{
				ClothesID:     &item.ID,
				Title:         item.Title,
				Size:          pickSize(item, request.Size),
				UnitPriceTiyn: item.PriceTiyn,
				Quantity:      1,
			}},
		}
		// GORM inserts the order, then its Items with order_id filled in, in this same transaction.
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		return tx.Create(&models.Payment{
			OrderID:    order.ID,
			AmountTiyn: order.TotalTiyn,
			Method:     "card",
			Status:     models.PaymentSucceeded,
		}).Error
	})

	if errors.Is(err, errOutOfStock) {
		c.HTML(http.StatusConflict, "error.html", gin.H{"error": "К сожалению, товар закончился"})
		return
	}
	if err != nil {
		log.Println("payment failed:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось оформить заказ, попробуйте ещё раз"})
		return
	}

	// Post/Redirect/Get: after a successful POST, send the browser to a GET page,
	// so pressing "refresh" doesn't submit the purchase a second time.
	c.Redirect(http.StatusSeeOther, "/orders")
}

// pickSize returns the requested size only if the item really has it, otherwise "".
// item.Size is either one size ("M") or a list with per-size stock ("S:2,M:0,L:3").
func pickSize(item models.Clothes, requested string) string {
	requested = strings.TrimSpace(requested)
	if requested == "" {
		return ""
	}
	for _, part := range strings.Split(item.Size, ",") {
		name, _, _ := strings.Cut(strings.TrimSpace(part), ":")
		if name == requested {
			return name
		}
	}
	return ""
}
