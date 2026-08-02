package middleware

import (
	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func AuthRequired(c *gin.Context) {
	// Retrieve session
	session, err := store.Get(c.Request, "session")
	if err != nil {

		fmt.Println("Error retrieving session:", err)
		c.Redirect(http.StatusFound, "/login")
		c.Abort()
		return
	}

	// Fetch user ID from session
	userID, ok := session.Values["userID"].(uint)

	if !ok || userID == 0 {
		c.Redirect(http.StatusFound, "/auth/login")
		c.Abort()
		return
	}

	var user models.User
	database.DB.First(&user, userID)
	if !user.Verified {
		c.HTML(http.StatusForbidden, "error.html", gin.H{"error": "Email is not verified"})
		c.Abort()
		return
	}

	// Store user ID in the context
	c.Set("userID", uint(userID))

	c.Next()
}
