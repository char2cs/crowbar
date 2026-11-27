package terminal

import (
	"fmt"
	"image/color"
	"os"
	"runtime"
	"strconv"
	"strings"
)

func ptyEnv() []string {
	base := os.Environ()
	overrides := map[string]string{
		"TERM":      "xterm-256color",
		"COLORTERM": "truecolor",
	}

	if lang := defaultLocale(base, runtime.GOOS); lang != "" {
		overrides["LANG"] = lang
	}

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

func FormatHexColor(c color.Color) string {
	if c == nil {
		return ""
	}
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", r>>8, g>>8, b>>8)
}

func ParseHexColor(s string) color.Color {
	if len(s) == 0 || s[0] != '#' {
		return nil
	}
	h := s[1:]
	if len(h) == 3 {
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
	if lang := defaultLocale(env, runtime.GOOS); lang != "" {
		env = append(env, "LANG="+lang)
	}
	return env
}
