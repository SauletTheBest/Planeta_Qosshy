package util

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Compiled once at startup instead of on every call.
// The domain ending may be up to 63 letters (.online, .travel); the old {2,4} rejected them.
var emailRe = regexp.MustCompile(`^[\w.+-]+@([\w-]+\.)+[\w-]{2,63}$`)

// NormalizeEmail turns "  Test@Mail.COM " into "test@mail.com".
// Every place that saves or looks up an email calls it, so one mailbox can't own
// two accounts and login works whatever letter case the user types.
func NormalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

func IsValidEmail(email string) bool {
	return len(email) <= 254 && emailRe.MatchString(email)
}

// ValidateUsername returns an error message, or "" if the name is acceptable.
// Characters are counted, not bytes: "Саулет" is 6 letters but 12 bytes.
func ValidateUsername(name string) string {
	if n := utf8.RuneCountInString(name); n < 3 || n > 20 {
		return "Имя пользователя должно быть от 3 до 20 символов."
	}
	return ""
}

// ValidatePassword returns an error message, or "" if the password is acceptable.
// The upper limit exists because bcrypt rejects passwords longer than 72 bytes.
func ValidatePassword(password string) string {
	if len(password) < 8 || len(password) > 72 {
		return "Пароль должен быть от 8 до 72 символов."
	}
	return ""
}
