package controllers

import (
	"net/http"

	"planeta_qosshy/database"
	"planeta_qosshy/models"

	"github.com/gin-gonic/gin"
)

// The old BuyClothes handler was removed: it wasn't routed anywhere and "bought" items
// without a payment, using the racy read-then-write stock update. Purchases go
// through ProcessPayment only.

func GetUserOrders(c *gin.Context) {
	userID := c.GetUint("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	// Preload("Items.Clothes") loads the orders, then their items, then the products of
	// those items: 3 queries in total, not 1 + N (one per order).
	var orders []models.Order
	if err := database.DB.Preload("Items.Clothes").
		Where("user_id = ?", userID).
		Order("created_at DESC").
		Find(&orders).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось загрузить заказы"})
		return
	}

	c.HTML(http.StatusOK, "orders.html", gin.H{
		"orders": orders,
	})
}
