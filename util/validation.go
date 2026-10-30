package util

import "regexp"

func IsValidEmail(email string) bool {
	re := regexp.MustCompile(`^[\w-\.+]+@([\w-]+\.)+[\w-]{2,4}$`)
	return re.MatchString(email)
}

// ValidatePassword returns an error message, or "" if the password is acceptable.
// The upper limit exists because bcrypt rejects passwords longer than 72 bytes.
func ValidatePassword(password string) string {
	if len(password) < 8 || len(password) > 72 {
		return "Password must be between 8 and 72 characters."
	}
	return ""
}
