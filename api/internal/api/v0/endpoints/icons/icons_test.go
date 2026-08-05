package icons_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/api/v0/endpoints/icons"
)

// TestContentType guards the fix for GitHub owner avatars that are SVG: Go's
// http.DetectContentType has no SVG signature and sniffs SVG as text/*, which an
// <img> refuses to render, so a fetched SVG icon silently degrades to the
// generated placeholder. ContentType must serve SVG as image/svg+xml while
// leaving real raster images on their sniffed image/* type.
//
// It moved here with the rest of the icon plumbing when projects grew the same
// editable icon repos already had: one implementation, one set of guards.
func TestContentType(t *testing.T) {
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
			if got := icons.ContentType(tc.data); got != tc.want {
				t.Fatalf("ContentType(%s) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

// TestIsSingleEmoji pins the grapheme-cluster rule: most real emoji are
// multi-codepoint sequences, and a code-point count would reject every one of
// them. A plain ASCII letter is not an emoji; a single non-ASCII mark is.
func TestIsSingleEmoji(t *testing.T) {
	for _, ok := range []string{"🦊", "❤️", "👨‍💻", "🇦🇷", "👍🏽", "★"} {
		if !icons.IsSingleEmoji(ok) {
			t.Errorf("IsSingleEmoji(%q) = false, want true", ok)
		}
	}
	for _, bad := range []string{"", "a", "ab", "🦊🦊", "hi"} {
		if icons.IsSingleEmoji(bad) {
			t.Errorf("IsSingleEmoji(%q) = true, want false", bad)
		}
	}
}
