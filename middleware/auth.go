package middleware

import (
	"net/http"

	"planeta_qosshy/database"
	"planeta_qosshy/models"

	"github.com/gin-gonic/gin"
)

// AuthRequired blocks unauthenticated requests and redirects to login.
// It does NOT block unverified users — email verification is informational only.
func AuthRequired(c *gin.Context) {
	session, err := GetSession(c.Request)
	if err != nil {
		// Usually an old cookie signed with a previous COOKIE_SECRET — not worth an error.
		log.WithError(err).Warn("cannot decode session cookie")
		c.Redirect(http.StatusFound, "/auth/login")
		c.Abort()
		return
	}

	userID, ok := session.Values["userID"].(uint)
	if !ok || userID == 0 {
		c.Redirect(http.StatusFound, "/auth/login")
		c.Abort()
		return
	}

	var user models.User
	database.DB.First(&user, userID)
	if user.ID == 0 {
		// User record deleted — clear session
		c.Redirect(http.StatusFound, "/auth/login")
		c.Abort()
		return
	}

	// The user is read from the DB on every request, so a block works at once:
	// the very next click after the admin pressed "block" ends up here.
	if user.IsBlocked() {
		session.Values = map[interface{}]interface{}{}
		session.Options.MaxAge = -1
		_ = session.Save(c.Request, c.Writer)
		c.Redirect(http.StatusFound, "/auth/login?blocked=1")
		c.Abort()
		return
	}

	c.Set("userID", userID)
	c.Set("role", user.Role)
	c.Set("username", user.Username)
	c.Set("isLoggedIn", true)
	c.Next()
}

// OptionalAuth reads the session and populates context values if the user is
// logged in, but always calls Next() — never redirects. Used for public pages.
func OptionalAuth(c *gin.Context) {
	session, err := GetSession(c.Request)
	if err != nil {
		c.Set("isLoggedIn", false)
		c.Next()
		return
	}

	userID, ok := session.Values["userID"].(uint)
	if !ok || userID == 0 {
		c.Set("isLoggedIn", false)
		c.Next()
		return
	}

	var user models.User
	database.DB.First(&user, userID)
	if user.ID == 0 || user.IsBlocked() {
		c.Set("isLoggedIn", false)
		c.Next()
		return
	}

	c.Set("userID", userID)
	c.Set("role", user.Role)
	c.Set("username", user.Username)
	c.Set("isLoggedIn", true)
	c.Next()
}
