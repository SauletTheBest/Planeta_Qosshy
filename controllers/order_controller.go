package controllers

import (
	"net/http"
	"strconv"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"github.com/gin-gonic/gin"
	_ "github.com/gorilla/sessions"
)

func BuyCar(c *gin.Context) {
	userID := c.GetUint("userID")
	if userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var form struct {
		CardName   string  `form:"card_name" binding:"required"`
		CardNumber string  `form:"card_number" binding:"required,len=16"`
		ExpiryDate string  `form:"expiry_date" binding:"required"`
		CVV        string  `form:"cvv" binding:"required,len=3"`
		CarID      uint    `form:"car_id" binding:"required"`
		Amount     float64 `form:"amount" binding:"required"`
	}
	println(form.CardName, form.CardNumber, form.ExpiryDate, form.CVV, form.Amount, userID)
	if err := c.ShouldBind(&form); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Invalid payment details"})
		return
	}

	carID, err := strconv.Atoi(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid car ID"})
		return
	}

	var car models.Car
	if err := database.DB.First(&car, carID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Car not found"})
		return
	}

	if car.Sold {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Car is already sold"})
		return
	}

	order := models.Order{
		UserID: uint(userID),
		CarID:  uint(carID),
	}

	database.DB.Model(&car).Update("Sold", true)
	database.DB.Create(&order)

	c.JSON(http.StatusOK, gin.H{"message": "Car purchased successfully"})
}

func GetUserOrders(c *gin.Context) {
	session, _ := store.Get(c.Request, "session")
	userID, exists := session.Values["user_id"].(uint)
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		return
	}

	var orders []models.Order
	database.DB.Preload("Car").Where("user_id = ?", userID).Find(&orders)

	c.HTML(http.StatusOK, "orders.html", gin.H{
		"users": orders,
	})
}
