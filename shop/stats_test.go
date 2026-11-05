package shop

import (
	"testing"
	"time"
)

func TestStartOfDayUsesShopTimeZone(t *testing.T) {
	almaty, err := time.LoadLocation("Asia/Almaty")
	if err != nil {
		t.Fatal(err)
	}
	// 20:30 UTC on 5 Nov is already 01:30 on 6 Nov in Kazakhstan (UTC+5).
	now := time.Date(2026, 11, 5, 20, 30, 0, 0, time.UTC)

	day := StartOfDay(now, almaty)
	if want := time.Date(2026, 11, 5, 19, 0, 0, 0, time.UTC); !day.Equal(want) {
		t.Errorf("start of day = %s, want %s (midnight 6 Nov in Almaty)", day.UTC(), want)
	}
	if got := StartOfDay(now, time.UTC); got.Day() != 5 {
		t.Errorf("in UTC it is still 5 Nov, got %s", got)
	}

	month := StartOfMonth(now, almaty)
	if want := time.Date(2026, 10, 31, 19, 0, 0, 0, time.UTC); !month.Equal(want) {
		t.Errorf("start of month = %s, want %s", month.UTC(), want)
	}
}
