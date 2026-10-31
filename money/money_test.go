package money

import "testing"

// Table-driven tests: one table of cases, one loop. Adding a case is one line.
func TestParseTenge(t *testing.T) {
	tests := []struct {
		in      string
		want    Tiyn
		wantErr bool
	}{
		{"15000", 1500000, false},
		{"15 000", 1500000, false},
		{"  15000  ", 1500000, false},
		{"1999.5", 199950, false},
		{"1999,50", 199950, false},
		{"0", 0, false},
		{"0.01", 1, false},
		{"", 0, true},
		{"-5", 0, true},
		{"+5", 0, true},
		{"5.-1", 0, true},
		{"1.234", 0, true},
		{"abc", 0, true},
		{"1e5", 0, true},
		{".5", 0, true},
		{"99999999999999999999", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseTenge(tt.in)
		if (err != nil) != tt.wantErr {
			t.Errorf("ParseTenge(%q) error = %v, wantErr %v", tt.in, err, tt.wantErr)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseTenge(%q) = %d, want %d", tt.in, got, tt.want)
		}
	}
}

func TestString(t *testing.T) {
	tests := []struct {
		in   Tiyn
		want string
	}{
		{0, "0"},
		{100, "1"},
		{1500000, "15 000"},
		{199950, "1 999,50"},
		{1, "0,01"},
		{100000000, "1 000 000"},
		{-150, "-1,50"},
	}
	for _, tt := range tests {
		if got := tt.in.String(); got != tt.want {
			t.Errorf("Tiyn(%d).String() = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestInputValueRoundTrip(t *testing.T) {
	for _, v := range []Tiyn{0, 1, 99, 100, 199950, 1500000} {
		back, err := ParseTenge(v.InputValue())
		if err != nil || back != v {
			t.Errorf("round trip of %d via %q gave %d, %v", v, v.InputValue(), back, err)
		}
	}
}
