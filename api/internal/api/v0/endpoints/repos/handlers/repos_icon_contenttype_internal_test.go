package handlers

import "testing"

// TestIconContentType guards the fix for GitHub owner avatars that are SVG: Go's
// http.DetectContentType has no SVG signature and sniffs SVG as text/*, which an
// <img> refuses to render, so a fetched SVG icon silently degrades to the
// generated placeholder. iconContentType must serve SVG as image/svg+xml while
// leaving real raster images on their sniffed image/* type.
func TestIconContentType(t *testing.T) {
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"bare svg", []byte(`<svg version="1.1" xmlns="http://www.w3.org/2000/svg"><rect/></svg>`), "image/svg+xml"},
		{"xml-prologue svg", []byte("<?xml version=\"1.0\"?>\n<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"), "image/svg+xml"},
		{"png", append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 32)...), "image/png"},
		{"jpeg", append([]byte("\xff\xd8\xff\xe0"), make([]byte, 32)...), "image/jpeg"},
		{"plain text stays text", []byte("just some text, no markup here at all"), "text/plain; charset=utf-8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := iconContentType(tc.data); got != tc.want {
				t.Fatalf("iconContentType(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}
