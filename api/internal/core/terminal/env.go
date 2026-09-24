package terminal

import (
	"image/color"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// ptyEnv returns the process environment augmented with the terminal capability
// vars that a GUI-launched daemon won't inherit from any shell session. Without
// TERM, readline-based programs (bash, zsh, Claude Code, etc.) fall back to
// dumb-terminal mode and disable history navigation and line editing.
func ptyEnv() []string {
	base := os.Environ()
	overrides := map[string]string{
		"TERM":      "xterm-256color",
		"COLORTERM": "truecolor",
	}

	// A GUI/launchd-launched daemon inherits launchd's minimal environment, which
	// carries NO locale — so the PTY shell falls back to the POSIX "C" locale and
	// mangles every non-ASCII byte (a typed argument or program output alike) into
	// U+FFFD. Default a UTF-8 locale when the inherited environment specifies none,
	// so "works in a real terminal, broken in the packaged app" glyph corruption
	// cannot happen. Never overrides a user's own explicit locale.
	if lang := defaultLocale(base, runtime.GOOS); lang != "" {
		overrides["LANG"] = lang
	}

	// Replace any existing TERM/COLORTERM entries, then append the rest.
	result := make([]string, 0, len(base)+len(overrides))
	for _, entry := range base {
		keep := true
		for key := range overrides {
			if len(entry) > len(key) && entry[:len(key)+1] == key+"=" {
				keep = false
				break
			}
		}
		if keep {
			result = append(result, entry)
		}
	}
	for k, v := range overrides {
		result = append(result, k+"="+v)
	}
	return result
}

// defaultLocale returns the UTF-8 LANG value ptyEnv should inject when the
// inherited environment carries NO locale at all, or "" when a locale is already
// present. It defaults ONLY when all three of LANG, LC_ALL and LC_CTYPE are
// unset, so a user's explicit locale is never overridden. macOS ships no
// C.UTF-8 locale, so darwin falls back to en_US.UTF-8; Linux (and CI) use the
// locale-independent C.UTF-8 — keyed off goos so the choice is portable.
func defaultLocale(
	base []string,
	goos string,
) string {
	for _, entry := range base {
		if strings.HasPrefix(entry, "LANG=") ||
			strings.HasPrefix(entry, "LC_ALL=") ||
			strings.HasPrefix(entry, "LC_CTYPE=") {
			return ""
		}
	}
	if goos == "darwin" {
		return "en_US.UTF-8"
	}
	return "C.UTF-8"
}

// DefaultLocaleForTest exposes the internal defaultLocale decision to the
// package's external unit tests so they can assert ptyEnv's per-GOOS UTF-8
// fallback for a synthetic environment without mutating the real process
// environment. It returns the LANG value ptyEnv would inject for the given base
// environment and GOOS, or "" when a locale is already set.
func DefaultLocaleForTest(
	base []string,
	goos string,
) string {
	return defaultLocale(base, goos)
}

// ParseHexColor converts the frontend's resolved CSS colour ("#rgb", "#rrggbb", or
// "#rrggbbaa" — the form resolve-css-color.ts emits) into a color.Color, or nil when the
// string is empty/unparseable. Alpha is dropped: the value feeds an OSC 11/10 default-colour
// report, which is RGB-only. A nil result is a safe no-op downstream — Session.SetTheme and
// SetHostTheme both leave a nil channel unchanged rather than resetting it.
//
// Exported because the hex string is the wire form on BOTH theme channels: the per-session
// WS frame handled below, and the host-theme REST push the API layer serves.
func ParseHexColor(s string) color.Color {
	if len(s) == 0 || s[0] != '#' {
		return nil
	}
	h := s[1:]
	if len(h) == 3 { // #rgb shorthand -> #rrggbb
		h = string([]byte{h[0], h[0], h[1], h[1], h[2], h[2]})
	}
	if len(h) != 6 && len(h) != 8 {
		return nil
	}
	v, err := strconv.ParseUint(h[:6], 16, 32)
	if err != nil {
		return nil
	}
	//nolint:gosec // v is a parsed 24-bit hex colour; these shifts/masks extract the R/G/B bytes and cannot overflow uint8.
	return color.RGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}

// withTerminalDefaults appends TERM=xterm-256color / COLORTERM=truecolor and a
// UTF-8 LANG only for keys the caller did not already provide, matching the real
// terminal engine's ptyEnv() seeding.
func withTerminalDefaults(env []string) []string {
	has := func(key string) bool {
		for _, kv := range env {
			if strings.HasPrefix(kv, key+"=") {
				return true
			}
		}
		return false
	}
	if !has("TERM") {
		env = append(env, "TERM=xterm-256color")
	}
	if !has("COLORTERM") {
		env = append(env, "COLORTERM=truecolor")
	}
	// The locale half of the same launchd-minimal-environment hazard ptyEnv() guards.
	// CreateCommand callers pass os.Environ() straight through (see the agent usecase),
	// so a GUI-launched daemon hands the vendor CLI an environment with NO locale at
	// all — and this backfill used to stop at TERM, which is why the interactive
	// terminal was fixed and agent chats were not.
	//
	// With no locale, CoreFoundation falls back to __CF_USER_TEXT_ENCODING, whose
	// script code is 0 (Mac OS Roman) on a default macOS account. Claude Code copies
	// via BOTH OSC 52 and a spawned pbcopy; we drop OSC 52 (finishOSC routes only
	// codes 0/1/2, and the frontend parses only OSC 7), so pbcopy's value is what
	// lands — and it reads the CLI's UTF-8 bytes as Mac Roman. "—" reaches the
	// pasteboard as "‚Äî" while the SCREEN stays correct, because the render path
	// never touches pbcopy. Every copy applies exactly one more round of it.
	//
	// defaultLocale returns "" the moment the caller set any of LANG/LC_ALL/LC_CTYPE,
	// so an explicit locale is never overridden.
	if lang := defaultLocale(env, runtime.GOOS); lang != "" {
		env = append(env, "LANG="+lang)
	}
	return env
}
