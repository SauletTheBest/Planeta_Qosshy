package middleware

import (
	"strings"

	"planeta_qosshy/database"
	"planeta_qosshy/models"

	"github.com/gin-gonic/gin"
)

// Me is what the shared header (templates/_site_parts.html) needs to know about the visitor.
type Me struct {
	LoggedIn  bool
	Username  string
	IsAdmin   bool
	CartCount int64 // "Корзина 3" in the menu
}

// Page renders an HTML page and adds what EVERY page needs: who is logged in, the
// cart counter, which menu item to highlight and a waiting flash message.
//
// Before, each handler passed isLoggedIn / username / isAdmin by hand (51 places).
// Some forgot, so the menu looked different from page to page. Now there is one place.
func Page(c *gin.Context, code int, name string, data gin.H) {
	if data == nil {
		data = gin.H{}
	}
	var me Me
	if c.GetBool("isLoggedIn") { // set by AuthRequired / OptionalAuth
		me = Me{
			LoggedIn: true,
			Username: c.GetString("username"),
			IsAdmin:  c.GetString("role") == models.RoleAdmin,
		}
		// One COUNT on the primary key (user_id, variant_id): cheap. A failure only
		// hides the counter, it must not break the page.
		database.DB.Model(&models.CartItem{}).Where("user_id = ?", c.GetUint("userID")).Count(&me.CartCount)
	}
	data["me"] = me
	if _, ok := data["section"]; !ok {
		data["section"] = sectionOf(c.Request.URL.Path)
	}
	if _, ok := data["flash"]; !ok {
		data["flash"] = PopFlash(c) // shown by the header on any page
	}
	c.HTML(code, name, data)
}

// sectionOf picks the menu item to highlight from the address.
func sectionOf(path string) string {
	switch {
	case path == "/":
		return "home"
	case strings.HasPrefix(path, "/clothes"):
		return "catalog"
	case strings.HasPrefix(path, "/cart"), strings.HasPrefix(path, "/payment"):
		return "cart"
	case strings.HasPrefix(path, "/orders"):
		return "orders"
	case strings.HasPrefix(path, "/profile"), strings.HasPrefix(path, "/helpdesk"):
		return "profile"
	case strings.HasPrefix(path, "/about"):
		return "about"
	}
	return ""
}
