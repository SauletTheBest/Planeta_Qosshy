package models

import (
	"reflect"
	"testing"
)

func TestParseSizeStock(t *testing.T) {
	tests := []struct {
		in      string
		want    []ClothesVariant
		wantErr bool
	}{
		{"", nil, false},
		{"Распродано", nil, false},
		{"M:1", []ClothesVariant{{Size: "M", Stock: 1}}, false},
		{"S:2, M:0, XL:5", []ClothesVariant{{Size: "S", Stock: 2}, {Size: "M", Stock: 0}, {Size: "XL", Stock: 5}}, false},
		{" 42 : 3 ", []ClothesVariant{{Size: "42", Stock: 3}}, false},
		{"One size:3", []ClothesVariant{{Size: "One size", Stock: 3}}, false},
		{"M", nil, true},          // no quantity
		{"M:-1", nil, true},       // negative
		{"M:abc", nil, true},      // not a number
		{"M:1, M:2", nil, true},   // duplicate
		{"<b>:1", nil, true},      // not a size name
		{"M:1,", nil, true},       // empty part
		{"M:99999999", nil, true}, // too many
		{`M":1`, nil, true},       // quote would break the admin page's CSS selector
	}
	for _, tt := range tests {
		got, err := ParseSizeStock(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseSizeStock(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if !reflect.DeepEqual(got, tt.want) {
			t.Errorf("ParseSizeStock(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
	}
}

func TestSizeSummaryOrderAndAvailability(t *testing.T) {
	c := Clothes{Variants: []ClothesVariant{
		{Size: "XL", Stock: 0}, {Size: "One size", Stock: 1}, {Size: "S", Stock: 2}, {Size: "M", Stock: 1},
	}}
	if got, want := c.SizeSummary(), "S:2, M:1, XL:0, One size:1"; got != want {
		t.Errorf("SizeSummary() = %q, want %q", got, want)
	}
	if got, want := c.AvailableSizes(), "S, M, One size"; got != want {
		t.Errorf("AvailableSizes() = %q, want %q", got, want)
	}
	if c.Variants[0].Size != "XL" {
		t.Errorf("SortedVariants must not reorder the original slice")
	}
	if got := (Clothes{}).AvailableSizes(); got != "Распродано" {
		t.Errorf("no variants: AvailableSizes() = %q", got)
	}
}
