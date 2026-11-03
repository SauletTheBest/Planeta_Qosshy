package controllers

import (
	"planeta_qosshy/database"
	"planeta_qosshy/middleware"
	"planeta_qosshy/models"
	"planeta_qosshy/util"
	"crypto/rand"
	"encoding/base64"
	"github.com/gin-gonic/gin"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net/http"
)

func generateToken() (string, error) {
	bytes := make([]byte, 32)
	_, err := rand.Read(bytes)
	if err != nil {
		return "", err
	}
	return base64.URLEncoding.EncodeToString(bytes), nil
}

func Register(c *gin.Context) {
	// Get form data
	username := c.PostForm("username")
	email := c.PostForm("email")
	password := c.PostForm("password")

	// Validate input
	if username == "" || email == "" || password == "" {
		c.HTML(http.StatusBadRequest, "register.html", gin.H{"error": "All fields are required."})
		return
	}
	if len(username) > 20 || len(username) < 3 {
		c.HTML(http.StatusConflict, "error.html", gin.H{"error": "Username is too short or too long. " +
			"The length should be between 3 and 20."})
		return
	}
	if len(email) > 250 || len(email) < 5 {
		c.HTML(http.StatusConflict, "error.html", gin.H{"error": "Email too long bruh. " +
			"The length should be between idk and 100. No one ever will see this"})
		return
	}
	if msg := util.ValidatePassword(password); msg != "" {
		c.HTML(http.StatusConflict, "error.html", gin.H{"error": msg})
		return
	}

	if !util.IsValidEmail(email) {
		c.HTML(http.StatusConflict, "error.html", gin.H{"error": "Email is invalid."})
		return
	}

	var existingUser models.User
	database.DB.Where("email = ?", email).First(&existingUser)
	if existingUser.ID != 0 {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Email already registered"})
		return
	}
	database.DB.Where("username = ?", username).First(&existingUser)
	if existingUser.ID != 0 {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Username taken"})
		return
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "register.html", gin.H{"error": "Failed to register user."})
		return
	}

	token, err := generateToken()
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Token generation failed"})
		return
	}

	user := models.User{
		Username:          username,
		Email:             email,
		Password:          string(hashedPassword),
		VerificationToken: token,
		Verified:          false,
	}

	if err := database.DB.Create(&user).Error; err != nil {
		c.HTML(http.StatusInternalServerError, "register.html", gin.H{"error": "Failed to register user."})
		return
	}

	// Try to send verification email — non-fatal if SMTP fails
	smtpErr := util.SendVerificationEmail(user.Email, token)
	if smtpErr != nil {
		log.Println("Warning: could not send verification email:", smtpErr)
		// Still redirect — user account was created successfully
		c.HTML(http.StatusOK, "verify.html", gin.H{
			"email":   user.Email,
			"warning": "Account created, but we couldn't send the verification email. Please contact support.",
		})
		return
	}

	c.HTML(http.StatusOK, "verify.html", gin.H{
		"email": user.Email,
	})
}

func Login(c *gin.Context) {

	email := c.PostForm("email")
	password := c.PostForm("password")

	if email == "" || password == "" {
		c.HTML(http.StatusBadRequest, "login.html", gin.H{"error": "Email and password are required."})
		return
	}

	var user models.User
	if err := database.DB.Where("email = ?", email).First(&user).Error; err != nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"error": "Invalid email or password."})
		return
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.Password), []byte(password)); err != nil {
		c.HTML(http.StatusUnauthorized, "login.html", gin.H{"error": "Invalid email or password."})
		return
	}

	// Only the user ID goes into the cookie; the role is always re-read from the DB by AuthRequired.
	session, _ := middleware.GetSession(c.Request)
	session.Values["userID"] = user.ID

	if err := session.Save(c.Request, c.Writer); err != nil {
		log.Println("session save error:", err)
		c.HTML(http.StatusInternalServerError, "login.html", gin.H{"error": "Login failed, please try again."})
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
