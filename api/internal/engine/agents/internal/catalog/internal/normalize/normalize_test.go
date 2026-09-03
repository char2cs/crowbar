package normalize_test

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog/internal/normalize"
)

func TestRedact_RemovesLocationsAndCredentials(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{"unix path", "see /Users/someone/secrets/key.pem now", "see [path] now"},
		{"home path", "see ~/config/creds.json now", "see [path] now"},
		{"windows path", `open C:\Users\someone\creds.txt now`, "open [path] now"},
		{"bearer token", "Bearer abcdef0123456789", "Bearer [redacted]"},
		{"api key assignment", "api_key: sk-notarealkey12345", "api_key=[redacted]"},
		{"openai style key", "sk-abcdefgh12345678", "[redacted]"},
		{"ordinary prose is untouched", "Review the diff carefully", "Review the diff carefully"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalize.Redact(tc.in))
		})
	}
}

func TestSource_DropsAnythingPathShaped(t *testing.T) {
	testCases := []struct {
		name string
		in   string
		want string
	}{
		{"plain name", "  superpowers  ", "superpowers"},
		{"unix path", "/opt/plugins/x", ""},
		{"windows path", `C:\plugins\x`, ""},
		{"windows drive alone", "C:", ""},
		{"home relative", "~/plugins", ""},
		{"backslash anywhere", `a\b`, ""},
		{"empty", "", ""},
		{"control characters stripped", "sup\x07er", "super"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, normalize.Source(tc.in))
		})
	}
}

func TestStripControls_KeepsNewlineAndTab(t *testing.T) {
	assert.Equal(t, "a\nb\tc", normalize.StripControls("a\nb\tc\x00\x07"))
}

func TestStripComposerControls_RemovesEveryControlCharacter(t *testing.T) {
	assert.Equal(t, "abc", normalize.StripComposerControls("a\nb\tc"))
}

func TestTruncateRunes_BoundsByVisibleCharacters(t *testing.T) {
	assert.Equal(t, "héllo", normalize.TruncateRunes("héllo world", 5))
	assert.Equal(t, "short", normalize.TruncateRunes("short", 99))
}

func TestTruncateBytes_NeverSplitsARune(t *testing.T) {
	got := normalize.TruncateBytes("aé", 2)

	assert.Equal(t, "a", got)
	assert.True(t, len(got) <= 2)
	assert.Equal(t, "abc", normalize.TruncateBytes("abc", 10))
}

func TestWarnings_DeduplicatesRedactsAndBounds(t *testing.T) {
	got := normalize.Warnings(nil, "one", "one", "", "at /Users/someone/x/y")

	assert.Equal(t, []string{"one", "at [path]"}, got)
}

func TestWarnings_StopsAtTheCeiling(t *testing.T) {
	existing := make([]string, normalize.MaxWarnings)
	for i := range existing {
		existing[i] = strings.Repeat("w", i+1)
	}

	got := normalize.Warnings(existing, "one more")

	assert.Len(t, got, normalize.MaxWarnings)
}

func TestWarnings_KeepsExistingEntries(t *testing.T) {
	got := normalize.Warnings([]string{"first"}, "second")

	assert.Equal(t, []string{"first", "second"}, got)
}
