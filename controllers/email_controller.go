package controllers

import (
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"planeta_qosshy/database"
	"planeta_qosshy/models"
	"planeta_qosshy/util"

	"github.com/gin-gonic/gin"
)

const (
	maxAttachments        = 3
	maxAttachmentSize     = 5 << 20  // 5 MB per file
	maxHelpdeskBody       = 16 << 20 // the whole form: 3 files + text + multipart overhead
	maxHelpdeskSubjectLen = 150
	maxHelpdeskMessageLen = 5000
)

// allowedAttachments maps what a file REALLY is (sniffed from its first bytes)
// to the extension we give it. The user's extension is not trusted at all.
var allowedAttachments = map[string]string{
	"image/jpeg":      ".jpg",
	"image/png":       ".png",
	"application/pdf": ".pdf",
}

// supportInbox is where helpdesk messages go: SUPPORT_EMAIL, or our own SMTP mailbox.
func supportInbox() string {
	if addr := os.Getenv("SUPPORT_EMAIL"); addr != "" {
		return addr
	}
	return os.Getenv("SMTP_USER")
}

func HelpdeskPage(c *gin.Context) {
	renderHelpdesk(c, http.StatusOK, gin.H{})
}

// renderHelpdesk shows the form; the reply address is always the account's own email.
func renderHelpdesk(c *gin.Context, status int, data gin.H) {
	var user models.User
	database.DB.First(&user, c.GetUint("userID"))
	data["email"] = user.Email
	c.HTML(status, "helpdesk.html", data)
}

func HelpdeskController(c *gin.Context) {
	// Stop reading after 16 MB. Without this limit a 2 GB upload would be received
	// completely and only then rejected.
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxHelpdeskBody)
	form, err := c.MultipartForm()
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			renderHelpdesk(c, http.StatusRequestEntityTooLarge, gin.H{"error": "Слишком большие файлы: до 5 МБ каждый, не больше трёх."})
			return
		}
		renderHelpdesk(c, http.StatusBadRequest, gin.H{"error": "Не удалось прочитать форму."})
		return
	}

	subject := strings.TrimSpace(c.PostForm("subject"))
	message := strings.TrimSpace(c.PostForm("message"))
	fail := func(status int, msg string) {
		renderHelpdesk(c, status, gin.H{"error": msg, "subject": subject, "message": message})
	}

	if subject == "" || message == "" {
		fail(http.StatusBadRequest, "Заполните тему и сообщение.")
		return
	}
	if utf8.RuneCountInString(subject) > maxHelpdeskSubjectLen || utf8.RuneCountInString(message) > maxHelpdeskMessageLen {
		fail(http.StatusBadRequest, fmt.Sprintf("Тема — до %d символов, сообщение — до %d.", maxHelpdeskSubjectLen, maxHelpdeskMessageLen))
		return
	}

	files := form.File["attachments"]
	if len(files) > maxAttachments {
		fail(http.StatusBadRequest, fmt.Sprintf("Можно приложить не больше %d файлов.", maxAttachments))
		return
	}

	var attachments []util.Attachment
	for i, fh := range files {
		data, err := readUpload(fh)
		if err != nil {
			fail(http.StatusBadRequest, fmt.Sprintf("Файл %q: %v", fh.Filename, err))
			return
		}
		// The extension is only part of the name: virus.exe renamed to photo.jpg is still
		// an .exe. DetectContentType looks at the first bytes of the file itself.
		ext, ok := allowedAttachments[http.DetectContentType(data)]
		if !ok {
			fail(http.StatusBadRequest, fmt.Sprintf("Файл %q: можно только JPG, PNG или PDF.", fh.Filename))
			return
		}
		// Our own name: the user's one may contain quotes, "../" or 500 characters.
		attachments = append(attachments, util.Attachment{
			Name: fmt.Sprintf("attachment-%d%s", i+1, ext),
			Data: data,
		})
	}

	var user models.User
	if err := database.DB.First(&user, c.GetUint("userID")).Error; err != nil {
		fail(http.StatusInternalServerError, "Пользователь не найден.")
		return
	}

	err = util.SendEmail(util.Email{
		To:          supportInbox(),
		ReplyTo:     user.Email, // support presses "Reply" and writes straight to the customer
		Subject:     "[Поддержка] " + subject,
		Body:        fmt.Sprintf("От: %s <%s>, id %d\n\n%s", user.Username, user.Email, user.ID, message),
		Attachments: attachments,
	})
	if err != nil {
		log.Println("helpdesk:", err)
		fail(http.StatusBadGateway, "Не удалось отправить письмо. Попробуйте позже.")
		return
	}

	renderHelpdesk(c, http.StatusOK, gin.H{"success": "Сообщение отправлено! Ответ придёт на ваш email."})
}

// readUpload reads one uploaded file into memory, refusing files over the size limit.
func readUpload(fh *multipart.FileHeader) ([]byte, error) {
	if fh.Size > maxAttachmentSize {
		return nil, errors.New("больше 5 МБ")
	}
	f, err := fh.Open()
	if err != nil {
		return nil, err
	}
	defer f.Close()

	// +1 byte: if we managed to read more than the limit, the header lied about the size.
	data, err := io.ReadAll(io.LimitReader(f, maxAttachmentSize+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxAttachmentSize {
		return nil, errors.New("больше 5 МБ")
	}
	return data, nil
}

func VerifyEmail(c *gin.Context) {
	token := c.Query("token")
	if token == "" {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "В ссылке нет кода подтверждения"})
		return
	}

	// The DB keeps only the hash, so we hash what came in the link and look for that.
	var user models.User
	database.DB.Where("verification_token = ?", util.HashToken(token)).First(&user)
	if user.ID == 0 {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Ссылка недействительна. Отправьте новое письмо из профиля."})
		return
	}
	if user.VerificationSentAt == nil || time.Since(*user.VerificationSentAt) > verificationTTL {
		c.HTML(http.StatusBadRequest, "error.html", gin.H{"error": "Ссылка устарела (действует 24 часа). Отправьте новое письмо из профиля."})
		return
	}

	// Only the two columns: Save(&user) would write back every field of the row.
	database.DB.Model(&user).Updates(map[string]any{"verified": true, "verification_token": ""})

	c.Redirect(http.StatusFound, "/auth/login?verified=1")
}
