package payments

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

const (
	SignatureHeader = "Qosshy-Signature" // "t=<unix time>,v1=<hex HMAC-SHA256>"

	EventSucceeded = "payment.succeeded"
	EventFailed    = "payment.failed"

	// DefaultTolerance: a signed webhook older than this is refused, so a captured
	// request can't be replayed days later.
	DefaultTolerance = 5 * time.Minute
)

var ErrBadSignature = errors.New("invalid webhook signature")

// Event is what the bank reports about a payment.
type Event struct {
	ID                string `json:"id"`
	Type              string `json:"type"`
	PaymentID         string `json:"payment_id"`
	MerchantReference string `json:"merchant_reference"`
	AmountTiyn        int64  `json:"amount_tiyn"`
	Currency          string `json:"currency"`
	CardLast4         string `json:"card_last4"`
	FailureReason     string `json:"failure_reason"`
	ReceiptURL        string `json:"receipt_url"`
}

// VerifySignature checks that the body was signed with our shared secret, recently.
// Anyone can send a POST to /payments/webhook; only the bank knows the secret.
func VerifySignature(secret []byte, header string, body []byte, now time.Time, tolerance time.Duration) error {
	var ts int64
	var sig []byte
	for _, part := range strings.Split(header, ",") {
		k, v, _ := strings.Cut(strings.TrimSpace(part), "=")
		switch k {
		case "t":
			ts, _ = strconv.ParseInt(v, 10, 64)
		case "v1":
			sig, _ = hex.DecodeString(v)
		}
	}
	if ts == 0 || len(sig) == 0 {
		return ErrBadSignature
	}
	if age := now.Sub(time.Unix(ts, 0)); age > tolerance || age < -tolerance {
		return fmt.Errorf("%w: timestamp is outside the allowed window", ErrBadSignature)
	}

	// The time is part of the signed text, so it can't be swapped for a fresh one.
	mac := hmac.New(sha256.New, secret)
	fmt.Fprintf(mac, "%d.", ts)
	mac.Write(body)

	// hmac.Equal compares in constant time: a normal == would leak, through response
	// timing, how many leading bytes of a forged signature were right.
	if !hmac.Equal(sig, mac.Sum(nil)) {
		return ErrBadSignature
	}
	return nil
}
