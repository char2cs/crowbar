// Package termprompt matches a live terminal screen against the blocking prompts
// a provider's descriptor declares it can paint.
//
// It answers one question — "is this CLI sitting behind a modal Crowbar has no
// channel to answer?" — and it answers it from declared strings only. A provider
// that declares nothing never matches, which is the whole degradation story: its
// chats behave exactly as they did before this package existed.
package termprompt

import (
	"strings"
	"unicode"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Match reports the prompt this screen is showing, if any.
//
// A SPECIFIC match wins over a generic one, regardless of declaration order. The
// two are different answers to different questions — "what is it blocked on" and
// "is it blocked" — and a descriptor that declares both (claude does: the trust
// dialog's own option line, plus the footer every one of its modals paints) must
// not have the answer decided by which line it happened to list first.
func Match(d *spec.Descriptor, screen string) (models.TerminalPrompt, bool) {
	if d == nil || len(d.TerminalPrompts) == 0 || screen == "" {
		return models.TerminalPrompt{}, false
	}
	haystack := squeeze(screen)
	if haystack == "" {
		return models.TerminalPrompt{}, false
	}

	var generic models.TerminalPrompt
	var found bool
	for _, p := range d.TerminalPrompts {
		needle := squeeze(p.Needle)
		if needle == "" || !strings.Contains(haystack, needle) {
			continue
		}
		if p.Kind != "" {
			return models.TerminalPrompt{Kind: p.Kind, Needle: p.Needle}, true
		}
		if !found {
			generic = models.TerminalPrompt{Needle: p.Needle}
			found = true
		}
	}
	return generic, found
}

// Declared reports whether this provider declares any terminal prompt at all —
// the capability read, so a caller can skip the screen read entirely for a
// provider that could never match.
func Declared(d *spec.Descriptor) bool {
	return d != nil && len(d.TerminalPrompts) > 0
}

// squeeze reduces text to lowercase letters and digits, dropping everything
// else — spaces, punctuation, box drawing, the newlines between screen rows.
//
// That is deliberately aggressive, and it is what makes a needle survive a TUI:
// the CLI centres its dialog, pads it, boxes it, and — at a narrow pane width —
// WRAPS it, so "Enter to confirm" can genuinely arrive as "Enter to" on one row
// and "confirm" on the next. A literal substring search finds neither. The same
// reduction is what the live integration harness matches with (see
// tests/kit's PTYTap.Contains), so the needles below inherit its verification
// rather than being re-guessed against a different comparison.
//
// The cost is a wider blast radius: a screen genuinely containing the letters of
// a needle in order, across word boundaries, matches. That is bounded by the
// caller's gates — a chat is only ever asked about while it is idle with no
// prompt of its own outstanding — and by the needles being footer lines a CLI
// paints only while it is blocked.
func squeeze(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}
