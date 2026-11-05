package controllers

import (
	"errors"
	"log"
	"net/http"
	"time"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/shop"
	"planeta_qosshy/util"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// resetTTL: how long a "forgot password" link works.
const resetTTL = time.Hour

var errBadResetLink = errors.New("invalid, used or expired reset link")

const badResetLinkMsg = "Ссылка для смены пароля недействительна, уже использована или устарела. Запросите новую."

func ForgotPasswordPage(c *gin.Context) {
	c.HTML(http.StatusOK, "forgot_password.html", gin.H{})
}

// ForgotPassword: POST /auth/forgot  email=...
func ForgotPassword(c *gin.Context) {
	email := util.NormalizeEmail(c.PostForm("email"))
	if !util.IsValidEmail(email) {
		c.HTML(http.StatusBadRequest, "forgot_password.html", gin.H{"error": "Неверный формат email.", "email": email})
		return
	}
	// The SAME answer whether the account exists or not. Otherwise this form would tell
	// a stranger which emails are registered here ("user enumeration").
	sent := func() {
		c.HTML(http.StatusOK, "forgot_password.html", gin.H{"sent": true, "email": email})
	}

	var user models.User
	if err := database.DB.Where("email = ?", email).First(&user).Error; err != nil || user.IsBlocked() {
		sent()
		return
	}

	raw, hash, err := util.NewToken()
	if err != nil {
		log.Println("forgot password: token:", err)
		sent()
		return
	}
	err = database.DB.Transaction(func(tx *gorm.DB) error {
		// Only the newest link works: asking again cancels the earlier ones.
		if err := tx.Where("user_id = ? AND used_at IS NULL", user.ID).Delete(&models.PasswordReset{}).Error; err != nil {
			return err
		}
		return tx.Create(&models.PasswordReset{
			UserID: user.ID, TokenHash: hash, ExpiresAt: time.Now().Add(resetTTL),
		}).Error
	})
	if err != nil {
		log.Println("forgot password: save:", err)
		sent()
		return
	}

	// In the background: if the answer came later only for existing accounts (because
	// talking to the mail server takes a second), the timing would give the secret away.
	link := shop.PublicURL + "/auth/reset?token=" + raw
	go func() {
		if err := util.SendPasswordResetEmail(user.Email, link); err != nil {
			log.Println("forgot password: email:", err)
		}
	}()
	sent()
}

// ResetPasswordPage: GET /auth/reset?token=... — the form for the new password.
func ResetPasswordPage(c *gin.Context) {
	raw := c.Query("token")
	if _, err := findReset(database.DB, raw, time.Now()); err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": badResetLinkMsg})
		return
	}
	c.HTML(http.StatusOK, "reset_password.html", gin.H{"token": raw})
}

// ResetPassword: POST /auth/reset  token, password, password2
func ResetPassword(c *gin.Context) {
	raw := c.PostForm("token")
	password, password2 := c.PostForm("password"), c.PostForm("password2")
	fail := func(status int, msg string) {
		c.HTML(status, "reset_password.html", gin.H{"token": raw, "error": msg})
	}
	if msg := util.ValidatePassword(password); msg != "" {
		fail(http.StatusBadRequest, msg)
		return
	}
	if password != password2 {
		fail(http.StatusBadRequest, "Пароли не совпадают.")
		return
	}

	now := time.Now().Truncate(time.Second)
	err := database.DB.Transaction(func(tx *gorm.DB) error {
		// FOR UPDATE: the same link submitted twice at the same moment is used only once.
		reset, err := findReset(tx.Clauses(clause.Locking{Strength: "UPDATE"}), raw, now)
		if err != nil {
			return err
		}
		if err := tx.Model(&reset).Update("used_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&models.User{}).Where("id = ?", reset.UserID).Updates(map[string]any{
			"password": util.HashPassword(password),
			// Every session started before now is logged out, including the one of
			// whoever may have learned the old password.
			"sessions_valid_after": now,
			// The person just proved that the mailbox is theirs.
			"verified": true,
		}).Error
	})
	if errors.Is(err, errBadResetLink) {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": badResetLinkMsg})
		return
	}
	if err != nil {
		log.Println("reset password:", err)
		fail(http.StatusInternalServerError, "Не удалось сменить пароль, попробуйте ещё раз.")
		return
	}
	c.Redirect(http.StatusSeeOther, "/auth/login?reset=1")
}

// findReset finds a link that exists, is unused and has not expired.
func findReset(db *gorm.DB, raw string, now time.Time) (models.PasswordReset, error) {
	var r models.PasswordReset
	if raw == "" {
		return r, errBadResetLink
	}
	err := db.Where("token_hash = ? AND used_at IS NULL AND expires_at > ?", util.HashToken(raw), now).First(&r).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return r, errBadResetLink
	}
	return r, err
}
