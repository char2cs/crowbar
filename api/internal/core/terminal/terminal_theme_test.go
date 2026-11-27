package terminal

import (
	"image/color"
	"testing"
)

func rgb8(c color.Color) (r, g, b uint8, ok bool) {
	if c == nil {
		return 0, 0, 0, false
	}
	rr, gg, bb, _ := c.RGBA()
	return uint8(rr >> 8), uint8(gg >> 8), uint8(bb >> 8), true
}

func TestParseHexColor(t *testing.T) {
	cases := []struct {
		in      string
		r, g, b uint8
		ok      bool
	}{
		{"#ffffff", 255, 255, 255, true},
		{"#000000", 0, 0, 0, true},
		{"#1e1e1e", 0x1e, 0x1e, 0x1e, true},
		{"#FAF9F5", 0xfa, 0xf9, 0xf5, true},
		{"#abc", 0xaa, 0xbb, 0xcc, true},
		{"#ffffffaa", 255, 255, 255, true},
		{"", 0, 0, 0, false},
		{"fff", 0, 0, 0, false},
		{"#12345", 0, 0, 0, false},
		{"#gggggg", 0, 0, 0, false},
	}
	for _, tc := range cases {
		r, g, b, ok := rgb8(ParseHexColor(tc.in))
		if ok != tc.ok || (ok && (r != tc.r || g != tc.g || b != tc.b)) {
			t.Errorf("ParseHexColor(%q) = (%d,%d,%d ok=%v), want (%d,%d,%d ok=%v)",
				tc.in, r, g, b, ok, tc.r, tc.g, tc.b, tc.ok)
		}
	}
}
