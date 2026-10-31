package payments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"testing"
	"time"
)

// sign does what the bank does (see planeta_qosshy_microservice/webhook.go).
func sign(secret, body []byte, t time.Time) string {
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%d.", t.Unix())
	mac.Write(body)
	return fmt.Sprintf("t=%d,v1=%s", t.Unix(), hex.EncodeToString(mac.Sum(nil)))
}

func TestVerifySignature(t *testing.T) {
	secret := []byte("webhook-secret-0123456789abcdef0123456789")
	body := []byte(`{"type":"payment.succeeded","amount_tiyn":1500050}`)
	now := time.Unix(1_800_000_000, 0)

	tests := []struct {
		name   string
		header string
		body   []byte
		ok     bool
	}{
		{"valid", sign(secret, body, now), body, true},
		{"a bit old but inside the window", sign(secret, body, now.Add(-4*time.Minute)), body, true},
		{"wrong secret", sign([]byte("another-secret-0123456789abcdef01234567"), body, now), body, false},
		{"body changed (amount)", sign(secret, body, now), []byte(`{"type":"payment.succeeded","amount_tiyn":1}`), false},
		{"replayed an hour later", sign(secret, body, now.Add(-time.Hour)), body, false},
		{"from the future", sign(secret, body, now.Add(time.Hour)), body, false},
		{"timestamp swapped", fmt.Sprintf("t=%d,%s", now.Unix(), sign(secret, body, now.Add(-time.Hour))[13:]), body, false},
		{"no header", "", body, false},
		{"garbage", "t=abc,v1=zz", body, false},
	}
	for _, tt := range tests {
		err := VerifySignature(secret, tt.header, tt.body, now, DefaultTolerance)
		if (err == nil) != tt.ok {
			t.Errorf("%s: err = %v, want ok=%v", tt.name, err, tt.ok)
		}
		if err != nil && !errors.Is(err, ErrBadSignature) {
			t.Errorf("%s: error should wrap ErrBadSignature, got %v", tt.name, err)
		}
	}
}
