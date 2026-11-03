package models

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"planeta_qosshy/money"
)

// The schema lives in migrations/*.sql; gorm tags here only describe the columns to GORM.
type Clothes struct {
	ID          uint       `gorm:"primaryKey" form:"id"`
	Title       string     `gorm:"not null" form:"title"`
	Brand       string     `gorm:"not null" form:"brand"`
	Category    string     `gorm:"not null;index" form:"category"` // e.g. Футболки, Худи, Куртки, Брюки, Обувь, Аксессуары
	Color       string     `gorm:"not null" form:"color"`          // e.g. Черный, Белый
	PriceTiyn   money.Tiyn `gorm:"not null" form:"-"`              // the form sends tenge; handlers convert with money.ParseTenge
	ImageURL    string     `gorm:"type:text" form:"image_url"`
	Description string     `gorm:"type:text" form:"description"`
	CreatedAt   time.Time  `gorm:"autoCreateTime"`

	// Stock and InStock are the total over all variants, kept up to date by a DB trigger
	// on clothes_variants. "->" makes them read-only for GORM, so Go code can't overwrite them.
	Stock   int  `gorm:"->" form:"-"`
	InStock bool `gorm:"->" form:"-"`

	Variants []ClothesVariant `gorm:"foreignKey:ClothesID" form:"-"`
}

// ClothesVariant is one size of a product with its own stock.
type ClothesVariant struct {
	ID        uint   `gorm:"primaryKey"`
	ClothesID uint   `gorm:"not null"`
	Size      string `gorm:"not null"`
	Stock     int    `gorm:"not null"`
}

// sizeOrder is how sizes are shown; unknown sizes go after these, alphabetically.
var sizeOrder = map[string]int{}

func init() {
	for i, s := range []string{"XS", "S", "M", "L", "XL", "XXL", "3XL",
		"36", "37", "38", "39", "40", "41", "42", "43", "44", "45"} {
		sizeOrder[s] = i
	}
}

// SortedVariants returns the variants in display order (XS, S, M … then shoe sizes).
func (c Clothes) SortedVariants() []ClothesVariant {
	vs := append([]ClothesVariant(nil), c.Variants...) // copy: don't reorder the caller's slice
	sort.SliceStable(vs, func(i, j int) bool {
		ri, iKnown := sizeOrder[vs[i].Size]
		rj, jKnown := sizeOrder[vs[j].Size]
		switch {
		case iKnown && jKnown:
			return ri < rj
		case iKnown != jKnown:
			return iKnown
		}
		return vs[i].Size < vs[j].Size
	})
	return vs
}

// SizeSummary is the "S:2, M:0" text the admin form and the size chips work with.
func (c Clothes) SizeSummary() string {
	parts := make([]string, 0, len(c.Variants))
	for _, v := range c.SortedVariants() {
		parts = append(parts, fmt.Sprintf("%s:%d", v.Size, v.Stock))
	}
	return strings.Join(parts, ", ")
}

// AvailableSizes lists the sizes that can be bought now: "S, M" or "Распродано".
func (c Clothes) AvailableSizes() string {
	var names []string
	for _, v := range c.SortedVariants() {
		if v.Stock > 0 {
			names = append(names, v.Size)
		}
	}
	if len(names) == 0 {
		return "Распродано"
	}
	return strings.Join(names, ", ")
}

// Variant finds the variant with the given size.
func (c Clothes) Variant(size string) (ClothesVariant, bool) {
	for _, v := range c.Variants {
		if v.Size == size {
			return v, true
		}
	}
	return ClothesVariant{}, false
}

var (
	ErrBadSizes   = errors.New("invalid size list")
	sizeNameRegex = regexp.MustCompile(`^[\p{L}\p{N}][\p{L}\p{N} .-]{0,15}$`)
)

const maxVariantStock = 1_000_000

// ParseSizeStock reads the admin form's "S:2, M:1" (or "Распродано" for nothing in stock).
// Sizes with 0 are allowed and kept; a size listed twice is an error, not silently merged.
func ParseSizeStock(s string) ([]ClothesVariant, error) {
	s = strings.TrimSpace(s)
	if s == "" || s == "Распродано" {
		return nil, nil
	}

	var variants []ClothesVariant
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		name, qty, ok := strings.Cut(strings.TrimSpace(part), ":")
		name = strings.TrimSpace(name)
		if !ok || !sizeNameRegex.MatchString(name) {
			return nil, fmt.Errorf("%w: %q", ErrBadSizes, part)
		}
		n, err := strconv.Atoi(strings.TrimSpace(qty))
		if err != nil || n < 0 || n > maxVariantStock {
			return nil, fmt.Errorf("%w: bad quantity in %q", ErrBadSizes, part)
		}
		if seen[name] {
			return nil, fmt.Errorf("%w: size %q listed twice", ErrBadSizes, name)
		}
		seen[name] = true
		variants = append(variants, ClothesVariant{Size: name, Stock: n})
	}
	return variants, nil
}
