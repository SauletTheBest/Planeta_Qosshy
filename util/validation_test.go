package util

import "testing"

func TestNormalizeEmail(t *testing.T) {
	if got := NormalizeEmail("  Test@Mail.COM \n"); got != "test@mail.com" {
		t.Fatalf("got %q", got)
	}
}

func TestIsValidEmail(t *testing.T) {
	valid := []string{"a@b.kz", "first.last+shop@mail.example.online", "user_1@sub-domain.travel"}
	invalid := []string{"", "plain", "a@b", "a@.kz", "a b@c.kz", "a@b.c"}
	for _, e := range valid {
		if !IsValidEmail(e) {
			t.Errorf("%q should be valid", e)
		}
	}
	for _, e := range invalid {
		if IsValidEmail(e) {
			t.Errorf("%q should be invalid", e)
		}
	}
}

func TestValidateUsername(t *testing.T) {
	if ValidateUsername("Саулет") != "" { // 6 letters, 12 bytes
		t.Error("Cyrillic name of 6 letters must be accepted")
	}
	if ValidateUsername("ab") == "" || ValidateUsername("abcdefghijklmnopqrstu") == "" {
		t.Error("2 and 21 characters must be rejected")
	}
}
