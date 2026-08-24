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
	if err := database.DB.Preload("Clothes").Where("user_id = ?", sessionUserID).Find(&purchases).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to fetch purchase history"})
		return
	}

	var purchasedClothes []models.Clothes
	for _, purchase := range purchases {
		purchasedClothes = append(purchasedClothes, purchase.Clothes)
	}

	chatID, chatStatus := GetChatStatus(fmt.Sprintf("%d", sessionUserID))

	c.HTML(http.StatusOK, "profile.html", gin.H{
		"user":             user,
		"purchasedClothes": purchasedClothes,
		"sessionUserID":    sessionUserID,
		"chatID":           chatID,
		"chatActive":       chatStatus == "active",
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
	if input.Username == "" || input.Email == "" {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Имя пользователя и Email обязательны для заполнения"})
		return
	}

	if !util.IsValidEmail(input.Email) {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Неверный формат Email"})
		return
	}

	updates := map[string]interface{}{
		"username": input.Username,
		"email":    input.Email,
	}

	if input.Password != "" {
		updates["password"] = util.HashPassword(input.Password)
	}

	result := database.DB.Model(&models.User{}).Where("id = ?", sessionUserID).Updates(updates)

	if result.Error != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось обновить профиль"})
		return
	}
	if result.RowsAffected == 0 {

		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "User not found or no changes made"})
		return
	}

	c.Redirect(http.StatusFound, "/profile")
}
