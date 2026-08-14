package controllers

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/util"

	"github.com/gin-gonic/gin"
)

func HelpdeskController(c *gin.Context) {
	email := c.PostForm("email")
	subject := c.PostForm("subject")
	message := c.PostForm("message")

	if email == "" || subject == "" || message == "" {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "All fields are required"})
		return
	}

	var attachments []string
	form, err := c.MultipartForm()
	if err != nil {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Failed to parse form data"})
		return
	}
	files := form.File["attachments"]

	for _, file := range files {
		ext := strings.ToLower(filepath.Ext(file.Filename))
		if ext != ".jpg" && ext != ".png" && ext != ".pdf" {
			c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Invalid file type"})
			return
		}

		filename := filepath.Join("uploads", file.Filename)
		if err := c.SaveUploadedFile(file, filename); err != nil {
			c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "File upload failed"})
			return
		}
		attachments = append(attachments, filename)
	}

	fullMessage := "From: " + email + "\n\n" + message
	// Send to the configured SMTP user (support inbox)
	supportEmail := os.Getenv("SMTP_USER")
	err = util.SendEmail(supportEmail, subject, fullMessage)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to send email. Please try again later."})
		return
	}

	c.HTML(http.StatusOK, "verify.html", gin.H{
		"email":   email,
		"warning": "Your message has been sent! We'll get back to you soon.",
	})
}

func VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Verification token is missing"})
		return
	}

	var user models.User
	database.DB.Where("verification_token = ?", token).First(&user)
	if user.ID == 0 {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Invalid or expired verification link"})
		return
	}

	if user.Verified {
		// Already verified — just redirect to login
		c.Redirect(http.StatusFound, "/auth/login")
		return
	}

	user.Verified = true
	user.VerificationToken = "" // Clear token after use
	database.DB.Save(&user)

	// Redirect to login with a success message
	c.Redirect(http.StatusFound, "/auth/login?verified=1")
}
