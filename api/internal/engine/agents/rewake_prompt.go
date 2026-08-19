package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/rewake"
)

// MatchRewakePrompt recovers the user's own text from a prompt this provider
// collected through its rewake channel, and reports false for anything else.
//
// IT MUST BE ASKED BEFORE MatchInjectedPrompt, and the ordering is the whole
// point of this function existing. The wrapper a rewake arrives in OPENS with the
// same `<task-notification>` document the provider's harness uses for its own
// background-subagent reports, so asked the other way round every message the
// user typed would match the injected-prompt needle and be filed under the
// harness's name — which is to say, deleted from the user's side of their own
// conversation.
//
// It is a package-level function taking an Agent, and NOT a method, for the same
// reason MatchInjectedPrompt is: the default is the load-bearing half. A nil, a
// caller's stub, an agent from a provider that declares no rewake — every one of
// them answers false, which means "not Crowbar's delivery", which is the
// behaviour that existed before this channel did. A method would make that
// fallback each implementation's to get right.
func MatchRewakePrompt(a Agent, prompt string) (string, bool) {
	matcher, ok := a.(rewakePromptMatcher)
	if !ok {
		return "", false
	}
	return matcher.matchRewakePrompt(prompt)
}

// RewakeSentinel is the Crowbar-authored string this provider must be told to
// prefix a collected prompt with, and RewakeSummary the one line it shows in its
// own terminal while doing so. Both are empty for a provider that declares no
// rewake channel.
//
// They are read back out of the descriptor rather than composed by the caller so
// that the value REGISTERED with the provider at spawn and the value MATCHED at
// ingest are one fact with one source. Two spellings of the sentinel is the one
// way this feature can break silently: delivery keeps working, and every prompt
// lands under the wrong name.
func RewakeSentinel(a Agent) string {
	matcher, ok := a.(rewakePromptMatcher)
	if !ok {
		return ""
	}
	return matcher.rewakeSentinel()
}

func RewakeSummary(a Agent) string {
	matcher, ok := a.(rewakePromptMatcher)
	if !ok {
		return ""
	}
	return matcher.rewakeSummary()
}

// RewakeWakeStatus is the exit status this provider's collector must leave with.
// Zero for a provider that declares no channel.
func RewakeWakeStatus(a Agent) int {
	matcher, ok := a.(rewakePromptMatcher)
	if !ok {
		return 0
	}
	return matcher.rewakeWakeStatus()
}

// rewakePromptMatcher is the unexported capability the three functions above look
// for. Unexported so it cannot be satisfied from outside this package: a caller
// that could implement it could also implement it as "every prompt is Crowbar's",
// which is a chat that attributes the harness's words to its user.
type rewakePromptMatcher interface {
	matchRewakePrompt(prompt string) (string, bool)
	rewakeSentinel() string
	rewakeSummary() string
	rewakeWakeStatus() int
}

func (a *agent) matchRewakePrompt(prompt string) (string, bool) {
	return rewake.Match(a.spec, prompt)
}

func (a *agent) rewakeSentinel() string { return rewake.Sentinel(a.spec) }

func (a *agent) rewakeSummary() string { return rewake.Summary(a.spec) }

func (a *agent) rewakeWakeStatus() int { return rewake.WakeStatus(a.spec) }
