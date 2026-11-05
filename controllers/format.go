package controllers

import (
	"strconv"
	"strings"
	"time"

	"planeta_qosshy/shop"
)

// FormatTime shows a time in the shop's time zone, "05.11.2026 14:30".
// Templates get it as {{datetime .CreatedAt}}; it accepts time.Time and *time.Time.
func FormatTime(v any) string {
	var t time.Time
	switch x := v.(type) {
	case time.Time:
		t = x
	case *time.Time:
		if x == nil {
			return "—"
		}
		t = *x
	default:
		return ""
	}
	if t.IsZero() {
		return "—"
	}
	return t.In(shop.Location).Format("02.01.2006 15:04")
}

// likePattern makes user text safe inside ILIKE. In SQL LIKE "%" and "_" are wildcards,
// so searching for "a_b" would also find "axb", and "%" alone would match everything.
// (This is not about SQL injection — the value is still passed as a parameter.)
func likePattern(s string) string {
	escape := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return "%" + escape.Replace(s) + "%"
}

// parseID reads a positive number from the URL, e.g. /admin/orders/:id.
func parseID(s string) (uint, bool) {
	n, err := strconv.ParseUint(s, 10, 32)
	if err != nil || n == 0 {
		return 0, false
	}
	return uint(n), true
}
