package controllers

import (
	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

// The default CheckOrigin rejects WebSocket connections opened by pages from other sites.
var upgrader = websocket.Upgrader{}

// wsClient remembers who is behind a connection, so a message is delivered
// only to the chat owner and to admins instead of every open browser tab.
type wsClient struct {
	conn    *websocket.Conn
	userID  uint
	isAdmin bool
}

// outgoing is a saved message plus the owner of its chat.
type outgoing struct {
	msg     models.Message
	ownerID uint
}

const maxMessageLen = 2000

var (
	clientsMu sync.Mutex // clients is used by many goroutines at once (one per connection)
	clients   = make(map[*websocket.Conn]*wsClient)
	broadcast = make(chan outgoing)
)

func HandleConnections(c *gin.Context) {
	userID := c.GetUint("userID")
	isAdmin := c.GetString("role") == "admin"
	username := c.GetString("username")

	conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		log.Println(err)
		return
	}
	defer conn.Close()

	clientsMu.Lock()
	clients[conn] = &wsClient{conn: conn, userID: userID, isAdmin: isAdmin}
	clientsMu.Unlock()
	defer removeClient(conn)

	for {
		var msg models.Message
		if err := conn.ReadJSON(&msg); err != nil {
			log.Println("Error reading message:", err)
			return
		}

		chat, ok := accessibleChat(msg.ChatID, userID, isAdmin)
		if !ok {
			continue
		}

		msg.Content = strings.TrimSpace(msg.Content)
		if msg.Content == "" || utf8.RuneCountInString(msg.Content) > maxMessageLen {
			continue
		}

		// The server decides who the sender is. The sender_id sent by the browser is
		// ignored, otherwise any user could post as "Admin".
		if isAdmin {
			msg.SenderID = "Admin"
		} else {
			msg.SenderID = username
		}
		msg.CreatedAt = time.Now()

		if err := database.DB.Create(&msg).Error; err != nil {
			log.Println("Ошибка сохранения в БД:", err)
			continue
		}

		broadcast <- outgoing{msg: msg, ownerID: uint(chat.UserID)}
	}
}

func removeClient(conn *websocket.Conn) {
	clientsMu.Lock()
	delete(clients, conn)
	clientsMu.Unlock()
}

func HandleMessages() {
	for out := range broadcast {
		// Pick recipients under the lock, write outside it,
		// so a slow client doesn't block new connections.
		clientsMu.Lock()
		var recipients []*wsClient
		for _, cl := range clients {
			if cl.isAdmin || cl.userID == out.ownerID {
				recipients = append(recipients, cl)
			}
		}
		clientsMu.Unlock()

		for _, cl := range recipients {
			if err := cl.conn.WriteJSON(out.msg); err != nil {
				log.Println("Error sending message:", err)
				cl.conn.Close()
				removeClient(cl.conn)
			}
		}
	}
}

// accessibleChat loads an active chat that the user may read and write:
// its owner or any admin. Missing, foreign and closed chats all return false.
func accessibleChat(chatID string, userID uint, isAdmin bool) (models.Chat, bool) {
	var chat models.Chat
	id, err := strconv.Atoi(chatID)
	if err != nil {
		return chat, false
	}
	if err := database.DB.First(&chat, id).Error; err != nil {
		return chat, false
	}
	if !isAdmin && uint(chat.UserID) != userID {
		return chat, false
	}
	return chat, strings.EqualFold(chat.Status, "active")
}

func StartChat(c *gin.Context) {
	// The chat is always opened for the logged-in user. The :id in the URL is ignored,
	// otherwise anyone could open chats on behalf of other users.
	userID := int(c.GetUint("userID"))

	var existingChat models.Chat
	err := database.DB.Where("user_id = ? AND status = 'active'", userID).First(&existingChat).Error

	if err == nil {
		c.Redirect(http.StatusSeeOther, fmt.Sprintf("/chat/%d", existingChat.ID))
		return
	}

	newChat := models.Chat{
		UserID:    userID,
		Status:    "active",
		CreatedAt: time.Now(),
	}

	if err := database.DB.Create(&newChat).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to start chat"})
		return
	}

	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/chat/%d", newChat.ID))
}

func ChatPage(c *gin.Context) {
	// Same 404 for "doesn't exist", "not yours" and "closed": we don't reveal which chat IDs exist.
	chat, ok := accessibleChat(c.Param("chatID"), c.GetUint("userID"), c.GetString("role") == "admin")
	if !ok {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Chat not found"})
		return
	}

	var messages []models.Message
	database.DB.Where("chat_id = ?", strconv.Itoa(chat.ID)).Order("created_at ASC").Find(&messages)

	c.HTML(http.StatusOK, "chat.html", gin.H{
		"chatID":   chat.ID,
		"messages": messages,
	})
}

func SendMessage(c *gin.Context) {
	chat, ok := accessibleChat(c.Param("chatID"), c.GetUint("userID"), c.GetString("role") == "admin")
	if !ok {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Chat not found"})
		return
	}

	content := strings.TrimSpace(c.PostForm("message"))
	if content == "" || utf8.RuneCountInString(content) > maxMessageLen {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Message is empty or too long"})
		return
	}

	message := models.Message{
		ChatID:   strconv.Itoa(chat.ID),
		SenderID: c.GetString("username"),
		Content:  content,
	}
	if err := database.DB.Create(&message).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to send message"})
		return
	}

	c.Redirect(http.StatusSeeOther, fmt.Sprintf("/chat/%d", chat.ID))
}

// APIGetActiveChat returns active chat status and history for widget
func APIGetActiveChat(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists || userIDVal == nil {
		c.JSON(http.StatusOK, gin.H{"isLoggedIn": false})
		return
	}

	userID := userIDVal.(uint)
	var user models.User
	database.DB.First(&user, userID)

	isAdmin := (user.Role == "admin")

	var chat models.Chat
	err := database.DB.Where("user_id = ? AND status = 'active'", userID).First(&chat).Error

	if err != nil {
		// Create a new active chat automatically
		chat = models.Chat{
			UserID:    int(userID),
			Status:    "active",
			CreatedAt: time.Now(),
		}
		if err := database.DB.Create(&chat).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create chat"})
			return
		}
	}

	var messages []models.Message
	database.DB.Where("chat_id = ?", strconv.Itoa(int(chat.ID))).Order("created_at ASC").Find(&messages)

	c.JSON(http.StatusOK, gin.H{
		"isLoggedIn": true,
		"isAdmin":    isAdmin,
		"username":   user.Username,
		"chatID":     chat.ID,
		"userID":     userID,
		"messages":   messages,
	})
}
