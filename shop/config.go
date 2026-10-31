// Package shop holds the order and payment rules, separate from HTTP: controllers read the
// request and render pages, shop decides what happens to orders, stock and payments.
package shop

import (
	"log"
	"os"
	"strings"
	"time"
)

var (
	// ReservationTTL is how long a pending order holds its items for the buyer to pay.
	ReservationTTL = 15 * time.Minute
	// ExpiryGrace is extra time before an unpaid order is cancelled, so a payment completed
	// in the last second still has time to arrive as a webhook.
	ExpiryGrace = time.Minute
	// ExpiryInterval is how often expired orders are looked for.
	ExpiryInterval = time.Minute
	// PublicURL is how buyers' browsers reach the shop (the bank sends them back here).
	PublicURL = "http://localhost:8080"
)

func MustConfigure() {
	ReservationTTL = durationEnv("ORDER_RESERVATION_TTL", ReservationTTL)
	ExpiryGrace = durationEnv("ORDER_EXPIRY_GRACE", ExpiryGrace)
	ExpiryInterval = durationEnv("ORDER_EXPIRY_INTERVAL", ExpiryInterval)
	if v := os.Getenv("PUBLIC_URL"); v != "" {
		PublicURL = strings.TrimRight(v, "/")
	} else if v := os.Getenv("VERIFICATION_ADDRESS"); v != "" {
		PublicURL = strings.TrimRight(v, "/")
	}
}

// durationEnv reads values like "15m" or "30s".
func durationEnv(key string, fallback time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		log.Fatalf("%s must be a positive duration like 15m or 30s, got %q", key, v)
	}
	return d
}
