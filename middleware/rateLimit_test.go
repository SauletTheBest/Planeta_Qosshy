package middleware

import (
	"testing"
	"time"
)

func TestIPLimiter(t *testing.T) {
	start := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	l := NewIPLimiter(10*time.Second, 3) // 3 at once, then one every 10 s

	for i := 1; i <= 3; i++ {
		if !l.Allow("1.1.1.1", start) {
			t.Fatalf("request %d of the burst was rejected", i)
		}
	}
	if l.Allow("1.1.1.1", start) {
		t.Fatal("4th request in the same instant must be rejected")
	}

	// Another IP has its own bucket: one noisy client does not block everybody.
	if !l.Allow("2.2.2.2", start) {
		t.Fatal("a different IP must not be affected")
	}

	// After 10 s exactly one new token has dripped in.
	later := start.Add(10 * time.Second)
	if !l.Allow("1.1.1.1", later) {
		t.Fatal("a token should be back after 10 s")
	}
	if l.Allow("1.1.1.1", later) {
		t.Fatal("only one token should be back after 10 s")
	}
}

func TestIPLimiterForgetsQuietIPs(t *testing.T) {
	start := time.Date(2026, 11, 5, 12, 0, 0, 0, time.UTC)
	l := NewIPLimiter(time.Second, 1)
	l.Allow("1.1.1.1", start)

	l.Allow("2.2.2.2", start.Add(time.Hour)) // triggers the cleanup
	if _, ok := l.visitors["1.1.1.1"]; ok {
		t.Fatal("an IP quiet for an hour should have been forgotten")
	}
	if len(l.visitors) != 1 {
		t.Fatalf("want 1 visitor left, got %d", len(l.visitors))
	}
}
