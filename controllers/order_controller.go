package controllers

import (
	"net/http"
	"strconv"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"github.com/gin-gonic/gin"
)

func BuyClothes(c *gin.Context) {
	userID := c.GetUint("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	clothesID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid clothes ID"})
		return
	}

	var item models.Clothes
	if err := database.DB.First(&item, clothesID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Clothing item not found"})
		return
	}

	if item.Stock <= 0 || !item.InStock {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Item is out of stock"})
		return
	}

	// Decrease stock counter by 1
	item.Stock -= 1
	if item.Stock == 0 {
		item.InStock = false
	}
	database.DB.Save(&item)

	order := models.Order{
		UserID:    userID,
		ClothesID: uint(clothesID),
	}
	database.DB.Create(&order)

	c.JSON(http.StatusOK, gin.H{"message": "Clothing item purchased successfully"})
}

func GetUserOrders(c *gin.Context) {
	userID := c.GetUint("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var orders []models.Order
	database.DB.Preload("Clothes").Where("user_id = ?", userID).Find(&orders)

	c.HTML(http.StatusOK, "orders.html", gin.H{
		"orders": orders,
	})
}
