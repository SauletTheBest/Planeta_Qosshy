package util

import (
	"fmt"
	"log"
	"os"
	"strconv"

	"gopkg.in/gomail.v2"
)

func smtpConfig() (host string, port int, user, pass string) {
	host = os.Getenv("SMTP_HOST")
	portStr := os.Getenv("SMTP_PORT")
	port, _ = strconv.Atoi(portStr)
	if port == 0 {
		port = 587
	}
	user = os.Getenv("SMTP_USER")
	pass = os.Getenv("SMTP_PASS")
	return
}

func SendEmail(to, subject, body string) error {
	host, port, user, pass := smtpConfig()

	m := gomail.NewMessage()
	m.SetHeader("From", user)
	m.SetHeader("To", to)
	m.SetHeader("Subject", subject)
	m.SetBody("text/plain", body)

	d := gomail.NewDialer(host, port, user, pass)

	if err := d.DialAndSend(m); err != nil {
		log.Println("Email send error:", err)
		return err
	}
	return nil
}

func SendVerificationEmail(email, token string) error {
	host, port, user, pass := smtpConfig()
	verificationAddress := os.Getenv("VERIFICATION_ADDRESS")

	mail := gomail.NewMessage()
	mail.SetHeader("From", user)
	mail.SetHeader("To", email)
	mail.SetHeader("Subject", "Verify Your Email — Planeta Qosshy")

	link := fmt.Sprintf("%s/auth/verify?token=%s", verificationAddress, token)
	mail.SetBody("text/html", fmt.Sprintf(`
<html>
<body style="font-family:sans-serif;background:#0f172a;color:#f8fafc;padding:40px;">
  <h1 style="color:#6366f1;">Verify Your Email</h1>
  <p>Thanks for signing up at <strong>Planeta Qosshy</strong>!</p>
  <p>Click the button below to verify your email address:</p>
  <a href="%s" style="display:inline-block;background:#6366f1;color:#fff;padding:12px 24px;border-radius:8px;text-decoration:none;font-weight:600;">
    Verify Email
  </a>
  <p style="margin-top:20px;color:#94a3b8;font-size:12px;">
    Or copy this link: %s
  </p>
</body>
</html>`, link, link))

	dialer := gomail.NewDialer(host, port, user, pass)
	return dialer.DialAndSend(mail)
}
