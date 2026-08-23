package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/promptorigin"
)

type InjectedPrompt struct {
	Kind   string
	Needle string
}

func MatchInjectedPrompt(a Agent, prompt string) (InjectedPrompt, bool) {
	matcher, ok := a.(injectedPromptMatcher)
	if !ok {
		return InjectedPrompt{}, false
	}
	return matcher.matchInjectedPrompt(prompt)
}

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
