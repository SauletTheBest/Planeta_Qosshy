package controllers

import (
	"net/http"
	"strings"
	"time"

	"planeta_qosshy/database"
	"planeta_qosshy/middleware"
	"planeta_qosshy/models"
	"planeta_qosshy/money"
	"planeta_qosshy/shop"

	"github.com/gin-gonic/gin"
)

// adminUserRow is one line of the users table: the account plus two numbers about it.
type adminUserRow struct {
	ID         uint
	Username   string
	Email      string
	Role       string
	Verified   bool
	BlockedAt  *time.Time
	CreatedAt  *time.Time
	OrderCount int64
	SpentTiyn  money.Tiyn
}

func (u adminUserRow) IsAdmin() bool   { return u.Role == models.RoleAdmin }
func (u adminUserRow) IsBlocked() bool { return u.BlockedAt != nil }

// AdminUsers: GET /admin/users?q=alice&page=2
func AdminUsers(c *gin.Context) {
	search := strings.TrimSpace(c.Query("q"))
	page := newPage(c.Query("page"), "", 25, 25)

	query := database.DB.Model(&models.User{})
	if search != "" {
		p := likePattern(search)
		query = query.Where("users.email ILIKE ? OR users.username ILIKE ?", p, p)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		adminFail(c, err)
		return
	}
	page.SetTotal(total)

	// Only the columns the page shows: the password hash has no business leaving the DB.
	// The two numbers are subqueries, so one query fills the whole table.
	var rows []adminUserRow
	if err := query.Select(`users.id, users.username, users.email, users.role, users.verified,
			users.blocked_at, users.created_at,
			(SELECT count(*) FROM orders o WHERE o.user_id = users.id) AS order_count,
			(SELECT COALESCE(SUM(o.total_tiyn), 0) FROM orders o
			  WHERE o.user_id = users.id AND o.status IN ?) AS spent_tiyn`, shop.EarnedStatuses).
		Order("users.id").Offset(page.Offset()).Limit(page.Size).
		Scan(&rows).Error; err != nil {
		adminFail(c, err)
		return
	}

	c.HTML(http.StatusOK, "ADMIN_users.html", gin.H{
		"users": rows, "q": search, "pager": page,
		"me": c.GetUint("userID"), "flash": middleware.PopFlash(c),
	})
}

// AdminSetRole: POST /admin/users/:id/role  role=admin|user
func AdminSetRole(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	role := c.PostForm("role")
	me := c.GetUint("userID")
	switch {
	case !ok:
		backToUsers(c, "error", "Пользователь не найден.")
		return
	case role != models.RoleUser && role != models.RoleAdmin:
		backToUsers(c, "error", "Неизвестная роль.")
		return
	case id == me:
		// Otherwise the last admin could demote themselves and nobody could open the panel again.
		backToUsers(c, "error", "Свою роль менять нельзя: так можно остаться без администратора.")
		return
	}

	res := database.DB.Model(&models.User{}).Where("id = ?", id).Update("role", role)
	if res.Error != nil || res.RowsAffected == 0 {
		backToUsers(c, "error", "Пользователь не найден.")
		return
	}
	middleware.Log().WithField("admin_id", me).WithField("user_id", id).WithField("role", role).Info("admin: role changed")
	backToUsers(c, "success", "Роль изменена.")
}

// AdminBlockUser: POST /admin/users/:id/block  action=block|unblock
func AdminBlockUser(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	me := c.GetUint("userID")
	block := c.PostForm("action") == "block"

	var user models.User
	if !ok || database.DB.First(&user, id).Error != nil {
		backToUsers(c, "error", "Пользователь не найден.")
		return
	}
	if id == me {
		backToUsers(c, "error", "Нельзя заблокировать самого себя.")
		return
	}
	if block && user.IsAdmin() {
		backToUsers(c, "error", "Сначала снимите с пользователя роль администратора.")
		return
	}

	var blockedAt any // nil = NULL in the DB
	if block {
		blockedAt = time.Now()
	}
	if err := database.DB.Model(&user).Update("blocked_at", blockedAt).Error; err != nil {
		adminFail(c, err)
		return
	}
	middleware.Log().WithField("admin_id", me).WithField("user_id", id).WithField("blocked", block).Info("admin: block changed")
	if block {
		backToUsers(c, "success", "Пользователь "+user.Username+" заблокирован: его сессии больше не работают.")
	} else {
		backToUsers(c, "success", "Пользователь "+user.Username+" разблокирован.")
	}
}

func backToUsers(c *gin.Context, kind, text string) {
	middleware.SetFlash(c, kind, text)
	c.Redirect(http.StatusSeeOther, "/admin/users")
}
