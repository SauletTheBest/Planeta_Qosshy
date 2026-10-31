package controllers

import (
	"errors"
	"html/template"
	"log"
	"net/http"

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
		"userID":    userID,
		"clothesID": clothesID,
		"amount":    item.Price,
		"item":      item,
	})
}

var errOutOfStock = errors.New("out of stock")

func ProcessPayment(c *gin.Context) {
	userID := c.GetUint("userID")
	// Only the item ID is read from the form. Card fields are not bound at all, and the
	// price always comes from the DB: a hidden "amount" input can be edited in the browser.
	var request struct {
		ClothesID uint `form:"clothes_id"`
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
	if err := database.DB.First(&item, request.ClothesID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Clothing item not found"})
		return
	}

	transaction := models.Transaction{
		Title:         item.Title,
		UserName:      user.Username,
		Price:         item.Price,
		Quantity:      1,
		TotalAmount:   item.Price,
		PaymentMethod: "card",
		Status:        "Paid", // the payment itself is still simulated; a real flow comes with the payment service
	}

	// One DB transaction: either stock goes down AND the payment and order are saved,
	// or nothing is saved. No more "stock decreased but the order is missing".
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

		if err := tx.Create(&transaction).Error; err != nil {
			return err
		}
		return tx.Create(&models.Order{UserID: user.ID, ClothesID: item.ID}).Error
	})

	if errors.Is(err, errOutOfStock) {
		c.JSON(http.StatusConflict, gin.H{"error": "Clothing item is out of stock"})
		return
	}
	if err != nil {
		log.Println("payment failed:", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment failed, please try again"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "Payment successful", "transaction": transaction})
}
