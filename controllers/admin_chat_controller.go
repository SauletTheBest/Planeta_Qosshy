package controllers

import (
	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type AdminChatItem struct {
	ID       int
	UserID   int
	Username string
	Email    string
	Status   string
}

func AdminChatList(c *gin.Context) {
	var chats []models.Chat
	database.DB.Order("created_at DESC").Find(&chats)

	var chatItems []AdminChatItem
	for _, chat := range chats {
		var u models.User
		database.DB.First(&u, chat.UserID)

		chatItems = append(chatItems, AdminChatItem{
			ID:       chat.ID,
			UserID:   chat.UserID,
			Username: u.Username,
			Email:    u.Email,
			Status:   chat.Status,
		})
	}

	c.HTML(http.StatusOK, "admin_chats.html", gin.H{
		"chats": chatItems,
	})
}

func AdminChat(c *gin.Context) {
	chatID := c.Param("chatID")
	var chat models.Chat

	if err := database.DB.Where("id = ?", chatID).First(&chat).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Chat not found"})
		return
	}

	var u models.User
	database.DB.First(&u, chat.UserID)

	var messages []models.Message
	database.DB.Where("chat_id = ?", chatID).Order("created_at ASC").Find(&messages)

	c.HTML(http.StatusOK, "admin_chat.html", gin.H{
		"chatID":   chat.ID,
		"username": u.Username,
		"email":    u.Email,
		"messages": messages,
	})
}

func AdminCloseChat(c *gin.Context) {
	chatID := c.Param("chatID")
	var chat models.Chat

	if err := database.DB.Where("id = ?", chatID).First(&chat).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Chat not found"})
		return
	}

	chat.Status = "inactive"
	chat.ClosedAt = time.Now()

	if err := database.DB.Save(&chat).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to update chat status"})
		return
	}

	c.Redirect(http.StatusFound, "/admin/chats")
}

func AdminDeleteChat(c *gin.Context) {
	chatID := c.Param("chatID")
	
	// Delete messages first
	database.DB.Where("chat_id = ?", chatID).Delete(&models.Message{})
	// Delete chat record
	database.DB.Where("id = ?", chatID).Delete(&models.Chat{})

	c.Redirect(http.StatusFound, "/admin/chats")
}

func AdminDeleteAllChats(c *gin.Context) {
	// Clear all messages and chats
	database.DB.Exec("DELETE FROM messages")
	database.DB.Exec("DELETE FROM chats")

	c.Redirect(http.StatusFound, "/admin/chats")
}

func AdminSendMessage(c *gin.Context) {
	chatID := c.Param("chatID")
	messageContent := c.PostForm("message")

	var chat models.Chat
	if err := database.DB.Where("id = ?", chatID).First(&chat).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Chat not found"})
		return
	}

	message := models.Message{
		ChatID:   chatID,
		SenderID: "Admin",
		Content:  messageContent,
	}
	database.DB.Create(&message)

	c.Redirect(http.StatusSeeOther, "/admin/chat/"+chatID)
}
