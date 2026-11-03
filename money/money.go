// Package money keeps amounts as an integer number of tiyn (1 ₸ = 100 tiyn).
//
// Why not float64: most decimal fractions have no exact binary form
// (0.1 + 0.2 == 0.30000000000000004), and these errors pile up in sums and totals.
// Integers are exact. Payment providers use the same idea ("amount in minor units").
package money

import (
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Tiyn is an amount of money in tiyn.
type Tiyn int64

const tiynPerTenge = 100

var ErrInvalidAmount = errors.New("invalid amount")

// FromTenge converts whole tenge to tiyn.
func FromTenge(tenge int64) Tiyn { return Tiyn(tenge * tiynPerTenge) }

// ParseTenge reads an amount typed by a person: "15000", "15 000", "1999.5", "1999,50".
// At most two digits after the separator; signs and other characters are rejected.
func ParseTenge(s string) (Tiyn, error) {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer(" ", "", " ", "", ",", ".").Replace(s)

	whole, frac, _ := strings.Cut(s, ".")
	if !isDigits(whole) || len(frac) > 2 || (frac != "" && !isDigits(frac)) {
		return 0, ErrInvalidAmount
	}

	w, err := strconv.ParseInt(whole, 10, 64)
	if err != nil || w > math.MaxInt64/tiynPerTenge-1 {
		return 0, ErrInvalidAmount
	}
	var f int64
	if frac != "" {
		f, _ = strconv.ParseInt(frac+strings.Repeat("0", 2-len(frac)), 10, 64) // "5" means 50 tiyn
	}
	return Tiyn(w*tiynPerTenge + f), nil
}

// String formats for people: 1500000 → "15 000", 199950 → "1 999,50".
// Templates call it automatically, so {{.PriceTiyn}} prints a ready price.
func (t Tiyn) String() string {
	sign := ""
	if t < 0 {
		sign, t = "-", -t
	}
	whole := groupThousands(strconv.FormatInt(int64(t)/tiynPerTenge, 10))
	if frac := int64(t) % tiynPerTenge; frac != 0 {
		return fmt.Sprintf("%s%s,%02d", sign, whole, frac)
	}
	return sign + whole
}

// InputValue formats for an <input type="number">: no spaces, a dot as separator.
func (t Tiyn) InputValue() string {
	if frac := int64(t) % tiynPerTenge; frac != 0 {
		return fmt.Sprintf("%d.%02d", int64(t)/tiynPerTenge, frac)
	}
	return strconv.FormatInt(int64(t)/tiynPerTenge, 10)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// groupThousands turns "1500000" into "1 500 000".
func groupThousands(digits string) string {
	var b strings.Builder
	for i, r := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(' ')
		}
		b.WriteRune(r)
	}
	return b.String()
}
