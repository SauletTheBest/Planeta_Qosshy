package util

import "strings"

// SplitList turns "a, b,,c" into [a b c]; an empty string gives nil.
// Used for comma-separated settings such as TRUSTED_PROXIES.
func SplitList(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	return out
}
