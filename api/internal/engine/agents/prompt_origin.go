package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/promptorigin"
)

// InjectedPrompt is a user-prompt hook whose text the provider's own HARNESS
// wrote — a background-subagent completion report is the measured case — rather
// than the human whose chat it lands in.
//
// Kind is Crowbar's own name for the injection, or empty when the provider can
// only say that this was not the user. Needle is the declaration that matched,
// carried so a log line can name the string a decision was taken on: this is a
// content match, and a content match that cannot be traced back to its needle is
// one nobody can audit when it gets a message wrong.
type InjectedPrompt struct {
	Kind   string
	Needle string
}

// MatchInjectedPrompt reports whether a user-prompt hook's text is one the
// provider's harness submits to its own model, rather than something the human
// sent.
//
// It is a package-level function taking an Agent, and NOT a method on the Agent
// interface, because its default is the load-bearing half of it: anything that
// is not a descriptor-backed agent — a nil, a caller's own stub, an agent from a
// provider that declares nothing — answers false, which means "the user's". A
// method would make that fallback each implementation's to get right, and every
// one of them getting it wrong looks like Crowbar putting words in a person's
// mouth. Expressed this way there is exactly one place the answer can come from
// and exactly one default.
//
// See spec.InjectedPromptSpec for what a declaration matches, and for the
// measurement showing that claude 2.1.234's user-prompt payload carries no field
// that could answer this structurally.
func MatchInjectedPrompt(a Agent, prompt string) (InjectedPrompt, bool) {
	matcher, ok := a.(injectedPromptMatcher)
	if !ok {
		return InjectedPrompt{}, false
	}
	return matcher.matchInjectedPrompt(prompt)
}

// injectedPromptMatcher is the unexported capability MatchInjectedPrompt looks
// for. Unexported so it cannot be satisfied from outside this package: a caller
// that could implement it could also implement it as "everything is injected",
// which is a chat whose every user message reads as the harness's.
type injectedPromptMatcher interface {
	matchInjectedPrompt(prompt string) (InjectedPrompt, bool)
}

func (a *agent) matchInjectedPrompt(prompt string) (InjectedPrompt, bool) {
	p, ok := promptorigin.Match(a.spec, prompt)
	if !ok {
		return InjectedPrompt{}, false
	}
	return InjectedPrompt{Kind: p.Kind, Needle: p.Needle}, true
}
