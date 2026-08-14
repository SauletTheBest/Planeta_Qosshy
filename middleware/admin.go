package middleware

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

// AdminMiddleware Middleware to check if the user is an admin
func RequireAdmin(c *gin.Context) {
	session, err := GetSession(c.Request)

	if err != nil {
		fmt.Println("Error retrieving session:", err)
		c.Redirect(http.StatusFound, "/auth/login")
		c.Abort()
		return
	}

	userRole, ok := session.Values["role"].(string)
	if !ok {
		c.HTML(http.StatusForbidden, "error.html", gin.H{"error": "Access denied"})
		c.Abort()
		return
	}

	fmt.Println(userRole)

	if userRole != "admin" {
		c.HTML(http.StatusForbidden, "error.html", gin.H{"error": "Access denied"})
		c.Abort()
		return
	}
	c.Next()
}
