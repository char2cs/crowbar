// Package promptorigin answers one question about a user-prompt hook: did the
// HUMAN write this, or did the provider's own harness?
//
// It answers it from declared strings only, and it answers "the human" whenever
// it is not certain. That asymmetry is the whole design. Getting it wrong in one
// direction puts the harness's words in the user's mouth — the defect this
// package exists to fix; getting it wrong in the other direction hides a message
// a person actually sent, which is a new way to be wrong that did not exist
// before. So a provider that declares nothing never matches, and its chats
// behave exactly as they did before this package existed.
//
// See spec.InjectedPromptSpec for what is being matched and, more importantly,
// why matching text is the only thing available: the payload carrying these
// prompts has no field that distinguishes them.
package promptorigin

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Match reports the declaration this prompt is an instance of, if any.
//
// A SPECIFIC match wins over a generic one, regardless of declaration order —
// the same rule termprompt applies, for the same reason: "which injection is
// this" and "is this an injection at all" are different questions, and a
// descriptor declaring both must not have the answer decided by which line it
// happened to list first.
//
// THE COMPARISON IS NOT TERMPROMPT'S, deliberately. That package squeezes both
// sides down to lowercase letters and digits before a substring search, because
// it matches a TUI SCREEN: its needle has to survive box drawing, centring,
// padding and — at a narrow pane width — a line wrap through the middle of a
// phrase. None of that happens here. This is the raw text of a hook payload, byte
// for byte as the CLI submitted it, with no renderer between the two; there is
// nothing to see through. And the reduction would be actively destructive:
// "<task-notification>" squeezes to "tasknotification", so a user asking about
// "task notification" handling would match, and the angle brackets that make the
// needle a piece of markup rather than an English phrase would be gone.
//
// So the comparison is: trim leading whitespace, then compare the needle as a
// case-sensitive PREFIX.
//
//   - Prefix, not substring, because the measured payload IS the document — it
//     opens with the tag. A substring search would also match a person quoting or
//     asking about one, and silencing a real message is the failure this package
//     must not introduce. The cost is that an injection some future release
//     prefixes with a preamble is missed, and a miss degrades to "recorded as the
//     user", which is exactly today's behaviour.
//   - Leading whitespace is trimmed because it is the one thing a delivery
//     channel can add without changing what was said.
//   - Case-sensitive because the needle is markup. Folding case would only widen
//     a match that is already as wide as it should be.
func Match(d *spec.Descriptor, prompt string) (spec.InjectedPromptSpec, bool) {
	if d == nil || len(d.InjectedPrompts) == 0 || prompt == "" {
		return spec.InjectedPromptSpec{}, false
	}
	body := strings.TrimLeft(prompt, " \t\r\n")
	if body == "" {
		return spec.InjectedPromptSpec{}, false
	}

	var generic spec.InjectedPromptSpec
	var found bool
	for _, p := range d.InjectedPrompts {
		if p.Needle == "" || !strings.HasPrefix(body, p.Needle) {
			continue
		}
		if p.Kind != "" {
			return p, true
		}
		if !found {
			generic = p
			found = true
		}
	}
	return generic, found
}

// Declared reports whether this provider declares any injected prompt at all —
// the capability read, so a caller can say "this provider's user_prompt hooks are
// all the user's" without walking an empty list per hook.
func Declared(d *spec.Descriptor) bool {
	return d != nil && len(d.InjectedPrompts) > 0
}
