package controllers

import (
	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/util"
	"github.com/gin-gonic/gin"
	"net/http"
	"path/filepath"
	"strings"
)

func HelpdeskController(c *gin.Context) {

	email := c.PostForm("email")
	subject := c.PostForm("subject")
	message := c.PostForm("message")

	// Validate input
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

	err = util.SendEmail("", subject, fullMessage)
	if err != nil {
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Failed to send email"})
		return
	}

	c.HTML(http.StatusOK, "error.html", gin.H{"error": "Email sent successfully!"})
}

func VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Token is required"})
		return
	}

	var user models.User
	database.DB.Where("verification_token = ?", token).First(&user)
	if user.ID == 0 {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Invalid token"})
		return
	}

	if user.Verified {
		c.HTML(http.StatusOK, "error.html", gin.H{"error": "Email already verified"})
		return
	}

	user.Verified = true
	database.DB.Save(&user)

	c.HTML(http.StatusOK, "error.html", gin.H{"error": "Email verified successfully"})
}
