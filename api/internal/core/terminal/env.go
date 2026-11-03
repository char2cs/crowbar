package terminal

import (
	"image/color"
	"os"
	"runtime"
	"strconv"
	"strings"
)

// ptyEnv returns the environment plus the terminal vars a GUI-launched daemon does not inherit; without
// TERM, readline programs drop to dumb mode with no history or line editing.
func ptyEnv() []string {
	base := os.Environ()
	overrides := map[string]string{
		"TERM":      "xterm-256color",
		"COLORTERM": "truecolor",
	}

	// A launchd-launched daemon carries no locale, so the shell falls back to "C" and mangles non-ASCII into
	// U+FFFD. Default a UTF-8 locale, never overriding the user's.
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

// defaultLocale returns the UTF-8 LANG to inject only when LANG, LC_ALL and LC_CTYPE are all unset, else
// "". darwin ships no C.UTF-8, so it gets en_US.UTF-8.
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

// ParseHexColor converts the frontend's "#rgb"/"#rrggbb"/"#rrggbbaa" into a color.Color, or nil (a safe
// no-op downstream). Alpha is dropped: OSC 11/10 reports are RGB-only. Exported because hex is the wire
// form on both the session WS frame and the host-theme REST push.
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
	// Same missing-locale hazard as ptyEnv, for commands: os.Environ() from a GUI daemon has no locale, and
	// macOS then falls back to Mac Roman, so a CLI's pbcopy mangles UTF-8 on the pasteboard. An explicit
	// locale is never overridden.
	if lang := defaultLocale(env, runtime.GOOS); lang != "" {
		env = append(env, "LANG="+lang)
	}
	return env
}
