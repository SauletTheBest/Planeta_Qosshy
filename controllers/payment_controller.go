package controllers

import (
	"bytes"
	"carSell/database"
	"carSell/models"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

func PaymentPage(c *gin.Context) {
	carID := c.Param("car_id")
	userID := c.GetUint("userID")

	var car models.Car

	if err := database.DB.First(&car, carID).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Car not found"})
		return
	}

	amount := car.Price
	t, err := template.ParseFiles("templates/payment.html")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	t.Execute(c.Writer, gin.H{
		"userID": userID,
		"carID":  carID,
		"amount": amount,
	})

}

func Payment(c *gin.Context) {
	userID := c.GetUint("userID")

	var form struct {
		CardName   string  `form:"card_name" binding:"required"`
		CardNumber string  `form:"card_number" binding:"required,len=16"`
		ExpiryDate string  `form:"expiry_date" binding:"required"`
		CVV        string  `form:"cvv" binding:"required,len=3"`
		CarID      uint    `form:"car_id" binding:"required"`
		Amount     float64 `form:"amount" binding:"required"`
	}

	if err := c.ShouldBind(&form); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Invalid payment details"})
		return
	}

	time.Sleep(2 * time.Second)

	var car models.Car
	if err := database.DB.First(&car, form.CarID).Error; err != nil || car.Sold {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Car not available or already sold"})
		return
	}

	car.Sold = true
	database.DB.Save(&car)

	order := models.Order{
		UserID:    userID,
		CarID:     car.ID,
		CreatedAt: time.Now(),
	}
	database.DB.Create(&order)

	c.Redirect(http.StatusSeeOther, "/orders")
}

type PaymentRequest struct {
	TransactionID uint    `json:"transaction_id"`
	Amount        float64 `json:"amount"`
	CardNumber    string  `json:"card_number"`
	Expiry        string  `json:"expiry"`
	Model         string
	UserName      string
	Price         float64
	Quantity      int
	TotalAmount   float64
	PaymentMethod string
	Status        string
	Email         string
	CreatedAt     time.Time `gorm:"autoCreateTime"`
}

func ProcessPayment(c *gin.Context) {
	userID, _ := c.Get("userID")
	var request struct {
		CardName   string  `form:"card_name"`
		CardNumber string  `form:"card_number"`
		ExpiryDate string  `form:"expiry_date"`
		CVV        string  `form:"cvv"`
		CarID      uint    `form:"car_id"`
		Amount     float64 `form:"amount"`
	}

	if err := c.ShouldBind(&request); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	fmt.Printf("User ID: %v\n", userID)
	fmt.Printf("Card Name: %s\n", request.CardName)
	fmt.Printf("Card Number: %s\n", request.CardNumber)
	fmt.Printf("Expiry Date: %s\n", request.ExpiryDate)
	fmt.Printf("CVV: %s\n", request.CVV)
	fmt.Printf("Car ID: %d\n", request.CarID)
	fmt.Printf("Amount: %.2f\n", request.Amount)

	var user models.User
	if err := database.DB.First(&user, userID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Пользователь не найден"})
		return
	}

	var car models.Car
	if err := database.DB.First(&car, request.CarID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Машина не найдена"})
		return
	}

	transaction := models.Transaction{
		Model:         car.Model,
		UserName:      request.CardName,
		Price:         car.Price,
		Quantity:      1,
		TotalAmount:   1,
		PaymentMethod: "card",
		Status:        "pending",
	}

	database.DB.Create(&transaction)

	paymentData := PaymentRequest{
		TransactionID: transaction.ID,
		Amount:        request.Amount,
		CardNumber:    request.CardNumber,
		Expiry:        request.ExpiryDate,
		Model:         car.Model,
		UserName:      user.Username,
		Price:         car.Price,
		Quantity:      1,
		TotalAmount:   1,
		PaymentMethod: "card",
		Status:        "pending",
		Email:         user.Email,
	}

	jsonData, _ := json.Marshal(paymentData)
	resp, err := http.Post("https://carsell-microservice.onrender.com/process-payment", "application/json", bytes.NewBuffer(jsonData))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment service unavailable"})
		return
	}
	defer resp.Body.Close()

	var response struct {
		Success bool `json:"success"`
	}
	json.NewDecoder(resp.Body).Decode(&response)

	if response.Success {
		err := database.DB.Model(&models.Transaction{}).Where("id = ?", transaction.ID).Update("Status", "Paid").Error
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment service unavailable: " + err.Error()})
		}
		err = database.DB.Model(&models.Car{}).Where("id = ?", car.ID).Update("sold", true).Error
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment service unavailable: " + err.Error()})
		}
		err = database.DB.Create(&models.Order{UserID: user.ID, CarID: car.ID}).Error
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment service unavailable: " + err.Error()})
		}
	} else {
		err := database.DB.Model(&models.Transaction{}).Where("id = ?", transaction.ID).Update("Status", "Declined").Error
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Payment service unavailable: " + err.Error()})
		}
	}

	c.JSON(http.StatusOK, gin.H{"transaction": transaction})
}
