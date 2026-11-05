package controllers

import (
	"crypto/rand"
	"encoding/base64"
	"log"
	"net/http"
	"strings"

	"planeta_qosshy/database"
	"planeta_qosshy/middleware"
	"planeta_qosshy/models"
	"planeta_qosshy/util"

	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
)

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

// duplicateUserMessage turns a UNIQUE-index error on users into words for the form.
// Constraint names differ between a fresh DB (users_email_key) and one first created
// by GORM AutoMigrate (uni_users_email), so we look for the column name inside.
func duplicateUserMessage(err error) (string, bool) {
	name, ok := database.UniqueViolation(err)
	switch {
	case !ok:
		return "", false
	case strings.Contains(name, "email"):
		return "Этот email уже зарегистрирован.", true
	case strings.Contains(name, "username"):
		return "Это имя пользователя уже занято.", true
	default:
		return "Такие данные уже используются.", true
	}
}

func Register(c *gin.Context) {
	username := strings.TrimSpace(c.PostForm("username"))
	email := util.NormalizeEmail(c.PostForm("email"))
	password := c.PostForm("password")

	// Show the form again with what the user typed (never the password) and the reason.
	fail := func(status int, msg string) {
		c.HTML(status, "register.html", gin.H{"error": msg, "username": username, "email": email})
	}

	if username == "" || email == "" || password == "" {
		fail(http.StatusBadRequest, "Заполните все поля.")
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
	if msg := util.ValidatePassword(password); msg != "" {
		fail(http.StatusBadRequest, msg)
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		fail(http.StatusInternalServerError, "Не удалось зарегистрироваться, попробуйте позже.")
		return
	}

	token, err := generateToken()
	if err != nil {
		fail(http.StatusInternalServerError, "Не удалось зарегистрироваться, попробуйте позже.")
		return
	}

	user := models.User{
		Username:          username,
		Email:             email,
		Password:          string(hashedPassword),
		VerificationToken: token,
		Verified:          false,
	}

	// No "SELECT first, then INSERT" any more: two sign-ups with the same email at the
	// same moment would both pass the SELECT. The UNIQUE index decides, we translate.
	if err := database.DB.Create(&user).Error; err != nil {
		if msg, ok := duplicateUserMessage(err); ok {
			fail(http.StatusConflict, msg)
			return
		}
		log.Println("register:", err)
		fail(http.StatusInternalServerError, "Не удалось зарегистрироваться, попробуйте позже.")
		return
	}

	// Try to send verification email — non-fatal if SMTP fails
	if err := util.SendVerificationEmail(user.Email, token); err != nil {
		log.Println("Warning: could not send verification email:", err)
		c.HTML(http.StatusOK, "verify.html", gin.H{
			"email":   user.Email,
			"warning": "Аккаунт создан, но письмо с подтверждением отправить не удалось. Напишите в поддержку.",
		})
		return
	}

	c.HTML(http.StatusOK, "verify.html", gin.H{
		"email": user.Email,
	})
}

func Login(c *gin.Context) {
	email := util.NormalizeEmail(c.PostForm("email"))
	password := c.PostForm("password")

	fail := func(status int, msg string) {
		c.HTML(status, "login.html", gin.H{"error": msg, "email": email})
	}

	if email == "" || password == "" {
		fail(http.StatusBadRequest, "Введите email и пароль.")
		return
	}

	// The same message for "no such email" and "wrong password":
	// otherwise the form tells a stranger which emails have accounts here.
	var user models.User
	if err := database.DB.Where("email = ?", email).First(&user).Error; err != nil {
		fail(http.StatusUnauthorized, "Неверный email или пароль.")
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		fail(http.StatusUnauthorized, "Неверный email или пароль.")
		return
	}

	// Only the user ID goes into the cookie; the role is always re-read from the DB by AuthRequired.
	session, _ := middleware.GetSession(c.Request)
	session.Values["userID"] = user.ID

	if err := session.Save(c.Request, c.Writer); err != nil {
		log.Println("session save error:", err)
		fail(http.StatusInternalServerError, "Не удалось войти, попробуйте ещё раз.")
		return
	}

	if user.Role == "admin" {
		c.Redirect(http.StatusFound, "/admin")
	} else {
		c.Redirect(http.StatusFound, "/clothes")
	}
}

func Logout(c *gin.Context) {
	session, _ := middleware.GetSession(c.Request)
	session.Values = map[interface{}]interface{}{}
	session.Options.MaxAge = -1 // tells the browser to delete the cookie

	// log.Fatal here used to stop the whole server because of one broken cookie
	if err := session.Save(c.Request, c.Writer); err != nil {
		log.Println("logout: session save error:", err)
	}

	c.Redirect(http.StatusFound, "/clothes")
}
