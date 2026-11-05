package controllers

import "testing"

func TestNewPage(t *testing.T) {
	tests := []struct {
		page, limit        string
		wantNumber, wantSz int
	}{
		{"", "", 1, 12},
		{"3", "24", 3, 24},
		{"0", "0", 1, 12},         // used to divide by zero
		{"-5", "-1", 1, 12},       // negative offset
		{"abc", "1000000", 1, 12}, // the whole catalog in one go
		{"2", "48", 2, 48},        // the maximum is allowed
		{"2", "49", 2, 12},
	}
	for _, tt := range tests {
		p := newPage(tt.page, tt.limit, 12, 48)
		if p.Number != tt.wantNumber || p.Size != tt.wantSz {
			t.Errorf("newPage(%q, %q) = page %d size %d, want %d %d",
				tt.page, tt.limit, p.Number, p.Size, tt.wantNumber, tt.wantSz)
		}
	}
}

func TestPageTotals(t *testing.T) {
	tests := []struct {
		total     int64
		size      int
		wantPages int
	}{
		{0, 12, 1},
		{1, 12, 1},
		{12, 12, 1},
		{13, 12, 2},
		{25, 12, 3}, // the old template showed "page 3 of 2" here
	}
	for _, tt := range tests {
		p := Page{Number: 1, Size: tt.size}
		p.SetTotal(tt.total)
		if p.TotalPages != tt.wantPages {
			t.Errorf("%d items by %d: %d pages, want %d", tt.total, tt.size, p.TotalPages, tt.wantPages)
		}
	}

	p := Page{Number: 3, Size: 12}
	p.SetTotal(25)
	if p.Offset() != 24 || !p.HasPrev() || p.HasNext() {
		t.Errorf("last page: offset %d prev %v next %v", p.Offset(), p.HasPrev(), p.HasNext())
	}
}
