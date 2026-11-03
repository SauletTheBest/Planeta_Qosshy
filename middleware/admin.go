package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// RequireAdmin must run after AuthRequired. It trusts only the role that AuthRequired
// loaded from the DB, never a role stored in the cookie, so a demoted admin loses access at once.
func RequireAdmin(c *gin.Context) {
	if c.GetString("role") != "admin" {
		c.HTML(http.StatusForbidden, "error.html", gin.H{"error": "Access denied"})
		c.Abort()
		return
	}
	c.Next()
}
