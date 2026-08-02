package controllers

import (
	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/util"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
	_ "reflect"
)

func Profile(c *gin.Context) {
	var user models.User
	sessionUserID := c.GetUint("userID")

	if err := database.DB.First(&user, sessionUserID).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "User not found"})
		return
	}

	var purchases []models.Order
	if err := database.DB.Preload("Car").Where("user_id = ?", sessionUserID).Find(&purchases).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to fetch purchase history"})
		return
	}

	var purchasedCars []models.Car
	for _, purchase := range purchases {
		purchasedCars = append(purchasedCars, purchase.Car)
	}

	chatID, chatStatus := GetChatStatus(fmt.Sprintf("%d", sessionUserID))

	c.HTML(http.StatusOK, "profile.html", gin.H{
		"user":          user,
		"purchasedCars": purchasedCars,
		"sessionUserID": sessionUserID,
		"chatID":        chatID,
		"chatActive":    chatStatus == "active",
	})
}

func GetChatStatus(userID string) (int, string) {
	var chat models.Chat

	err := database.DB.Where("user_id = ? AND status = 'active'", userID).First(&chat).Error

	if err != nil {
		return 0, "inactive"
	}

	return chat.ID, "active"
}

func UpdateProfilePage(c *gin.Context) {
	var user models.User
	sessionUserID := c.GetUint("userID")

	if err := database.DB.First(&user, sessionUserID).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "User not found"})
		return
	}
	user.Password = ""
	c.HTML(http.StatusOK, "edit_user.html", gin.H{"user": user})
}

func UpdateProfile(c *gin.Context) {
	sessionUserID := c.GetUint("userID")
	if sessionUserID == 0 {
		c.HTML(http.StatusUnauthorized, "error.html", gin.H{"error": "Unauthorized access"})
		return
	}

	var input struct {
		Username string `form:"username"`
		Email    string `form:"email"`
		Password string `form:"password"`
	}

	if err := c.ShouldBind(&input); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": err.Error()})
		return
	}
	print(input.Username, "|", input.Email, "|", input.Password)
	if input.Username == "" || input.Email == "" || input.Password == "" {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Username or email and password are required"})
		return
	}

	if !util.IsValidEmail(input.Email) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid email format"})
		return
	}

	hashedPassword := util.HashPassword(input.Password)

	result := database.DB.Model(&models.User{}).Where("id = ?", sessionUserID).Updates(models.User{
		Username: input.Username,
		Email:    input.Email,
		Password: hashedPassword,
	})

	if result.Error != nil {

		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to update profile"})
		return
	}
	if result.RowsAffected == 0 {

		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "User not found or no changes made"})
		return
	}

	c.Redirect(http.StatusFound, "/profile")
}
