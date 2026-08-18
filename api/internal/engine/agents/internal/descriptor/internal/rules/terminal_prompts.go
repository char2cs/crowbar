package rules

import (
	"strings"
	"unicode"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type terminalPrompts struct{}

func (terminalPrompts) Name() string { return "terminal_prompts" }

func (terminalPrompts) Check(d *spec.Descriptor) error {
	for i, p := range d.TerminalPrompts {
		if !hasContent(p.Needle) {
			return invalid(d.ID, "terminal_prompts[%d]: needle must contain letters or digits", i)
		}
		if p.Kind == "" {
			continue
		}
		if _, ok := spec.TerminalPromptKinds[p.Kind]; !ok {
			// Rejected rather than downgraded to the generic case. A typo that
			// silently became "we only know that something is up" would look
			// exactly like a working descriptor forever, and the one thing this
			// feature exists to prevent is a state that explains nothing.
			return invalid(d.ID, "terminal_prompts[%d]: unknown kind %q", i, p.Kind)
		}
	}
	return nil
}

// hasContent reports whether a needle survives the matcher's own reduction to
// letters and digits. A needle of pure punctuation ("· ⏎") reduces to the empty
// string, which every screen contains — so it would report every idle chat as
// blocked. Refused here rather than skipped at match time, because a descriptor
// that cannot do what it says is a descriptor error.
func hasContent(s string) bool {
	return strings.ContainsFunc(s, func(r rune) bool {
		return unicode.IsLetter(r) || unicode.IsDigit(r)
	})
}
