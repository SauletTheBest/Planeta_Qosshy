package controllers

import (
	
	"html/template"
	"net/http"
	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"github.com/gin-gonic/gin"
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
		"userID":    userID,
		"clothesID": clothesID,
		"amount":    item.Price,
		"item":      item,
	})
}

func ProcessPayment(c *gin.Context) {
	userID, _ := c.Get("userID")
	var request struct {
		CardName   string  `form:"card_name"`
		CardNumber string  `form:"card_number"`
		ExpiryDate string  `form:"expiry_date"`
		CVV        string  `form:"cvv"`
		ClothesID  uint    `form:"clothes_id"`
		Amount     float64 `form:"amount"`
	}

	if err := c.ShouldBind(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request parameters"})
		return
	}

	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "User not found"})
		return
	}

	var item models.Clothes
	if err := database.DB.First(&item, request.ClothesID).Error; err != nil || item.Stock <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Clothing item is out of stock"})
		return
	}

	transaction := models.Transaction{
		Title:         item.Title,
		UserName:      user.Username,
		Price:         item.Price,
		Quantity:      1,
		TotalAmount:   item.Price,
		PaymentMethod: "card",
		Status:        "pending",
	}
	database.DB.Create(&transaction)

	// Simulate payment processing / gateway update
	database.DB.Model(&models.Transaction{}).Where("id = ?", transaction.ID).Update("Status", "Paid")

	// Update item inventory stock
	item.Stock -= 1
	if item.Stock <= 0 {
		item.InStock = false
	}
	database.DB.Save(&item)

	// Create user order record
	database.DB.Create(&models.Order{
		UserID:    user.ID,
		ClothesID: item.ID,
	})

	c.JSON(http.StatusOK, gin.H{"message": "Payment successful", "transaction": transaction})
}
