package controllers

import (
	"fmt"
	"log"
	"net/http"
	"strings"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/util"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func Profile(c *gin.Context) {
	var user models.User
	sessionUserID := c.GetUint("userID")

	if err := database.DB.First(&user, sessionUserID).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "User not found"})
		return
	}

	// Every item the user has bought, newest first. The JOIN is needed because
	// order_items has no user_id: ownership is known only through orders.
	var purchases []models.OrderItem
	if err := database.DB.Preload("Clothes").
		Joins("JOIN orders ON orders.id = order_items.order_id").
		Where("orders.user_id = ?", sessionUserID).
		Order("orders.created_at DESC").
		Find(&purchases).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to fetch purchase history"})
		return
	}

	chatID, chatStatus := GetChatStatus(fmt.Sprintf("%d", sessionUserID))

	c.HTML(http.StatusOK, "profile.html", gin.H{
		"user":          user,
		"purchases":     purchases,
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
	var user models.User
	if err := database.DB.First(&user, c.GetUint("userID")).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Пользователь не найден"})
		return
	}

	username := strings.TrimSpace(c.PostForm("username"))
	email := util.NormalizeEmail(c.PostForm("email"))
	newPassword := c.PostForm("password")
	currentPassword := c.PostForm("current_password")

	// Show the form again with the typed values and the reason (instead of a bare error page).
	fail := func(status int, msg string) {
		shown := user
		shown.Username, shown.Email, shown.Password = username, email, ""
		c.HTML(status, "edit_user.html", gin.H{"user": shown, "error": msg})
	}

	if username == "" || email == "" {
		fail(http.StatusBadRequest, "Имя пользователя и email обязательны.")
		return
	}
	if msg := util.ValidateUsername(username); msg != "" {
		fail(http.StatusBadRequest, msg)
		return
	}
	if !util.IsValidEmail(email) {
		fail(http.StatusBadRequest, "Неверный формат email.")
		return
	}

	emailChanged := email != user.Email

	// A new email or password = a new owner of the account. Ask for the current password,
	// so someone at your unlocked computer can't quietly take the account over.
	if emailChanged || newPassword != "" {
		if currentPassword == "" ||
			bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(currentPassword)) != nil {
			fail(http.StatusForbidden, "Чтобы сменить email или пароль, введите текущий пароль.")
			return
		}
	}

	updates := map[string]interface{}{
		"username": username,
		"email":    email,
	}

	if newPassword != "" {
		if msg := util.ValidatePassword(newPassword); msg != "" {
			fail(http.StatusBadRequest, msg)
			return
		}
		updates["password"] = util.HashPassword(newPassword)
	}

	// The new address has not been proven to belong to this person yet.
	var token string
	if emailChanged {
		var err error
		if token, err = generateToken(); err != nil {
			fail(http.StatusInternalServerError, "Не удалось обновить профиль.")
			return
		}
		updates["verified"] = false
		updates["verification_token"] = token
	}

	if err := database.DB.Model(&models.User{}).Where("id = ?", user.ID).Updates(updates).Error; err != nil {
		if msg, ok := duplicateUserMessage(err); ok {
			fail(http.StatusConflict, msg)
			return
		}
		fail(http.StatusInternalServerError, "Не удалось обновить профиль.")
		return
	}

	if emailChanged {
		if err := util.SendVerificationEmail(email, token); err != nil {
			log.Println("profile: could not send verification email:", err)
		}
	}

	// 303 See Other: after a POST the browser must load /profile with GET.
	c.Redirect(http.StatusSeeOther, "/profile")
}
