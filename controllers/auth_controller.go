package controllers

import (
	"carSell/database"
	"carSell/models"
	"carSell/util"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/sessions"
	"golang.org/x/crypto/bcrypt"
	"log"
	"net/http"
	"os"
)

var store = sessions.NewCookieStore([]byte(os.Getenv("COOKIE_SECRET")))

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

	println(username)
	println(email)
	println(password)
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
	if len(password) > 100 || len(password) < 3 {
		c.HTML(http.StatusConflict, "error.html", gin.H{"error": "Password is too short or too long. Should be between 3 and 100"})
		return
	}

	if !util.IsValidEmail(email) {
		c.HTML(http.StatusConflict, "error.html", gin.H{"error": "Email is invalid."})
		return
	}

	println("password length: ", len(password))

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

	err = util.SendVerificationEmail(user.Email, token)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to send verification email"})
		return
	}

	c.Redirect(http.StatusFound, "/auth/login")
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

	session, _ := store.Get(c.Request, "session")
	fmt.Println(user.Role, " ", user.ID)
	session.Values["userID"] = user.ID
	session.Values["role"] = user.Role

	err := session.Save(c.Request, c.Writer)
	if err != nil {
		println("session error", err.Error())
		return
	}

	if user.Role == "admin" {
		c.Redirect(http.StatusFound, "/admin")
	} else {
		c.Redirect(http.StatusFound, "/cars")
	}
}

func Logout(c *gin.Context) {
	session, _ := store.Get(c.Request, "session")
	delete(session.Values, "userID")
	err := session.Save(c.Request, c.Writer)
	if err != nil {
		log.Fatal(err)
		return
	}

	c.Redirect(http.StatusFound, "/auth/login")
}
