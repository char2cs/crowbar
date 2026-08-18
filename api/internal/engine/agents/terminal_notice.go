package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/termprompt"
)

// TerminalNotice is a CLI caught explaining, on its own screen and nowhere else,
// that it is not going to do what it was asked. See models.TerminalNotice.
type TerminalNotice = models.TerminalNotice

// Terminal-notice kinds — Crowbar's own name for a message a CLI paints instead
// of finishing a turn. Unlike a prompt kind this is never empty: a descriptor
// cannot declare a notice without one.
const (
	TerminalNoticeUsageLimit = spec.TerminalNoticeUsageLimit
)

// NoticeMatcher is the optional half of an Agent: reading a provider NOTICE off a
// screen.
//
// It is a capability interface rather than a method on Agent because exactly one
// caller needs it — the stall detector — while every other consumer of an Agent
// (spawn, hooks, telemetry, catalogues) never asks the question at all. A caller
// that wants it type-asserts for it and treats a negative assertion the same way
// it treats a provider declaring no notices: as silence. Both are the identical
// answer, "this agent has nothing to say about its screen", so there is no
// degradation path to get wrong.
type NoticeMatcher interface {
	// MatchTerminalNotice reports whether this CLI's visible screen carries a
	// notice its descriptor declares, and captures the provider's own sentence
	// when it does.
	//
	// screen is plain text already rendered from the daemon's own VT model — the
	// same read the terminal-prompt match is made against, and deliberately so:
	// one screen read answers both questions, because rendering a cell grid to
	// text is the only expensive thing either of them does.
	MatchTerminalNotice(screen string) (TerminalNotice, bool)
}

// MatchTerminalNotice implements NoticeMatcher.
func (a *agent) MatchTerminalNotice(screen string) (TerminalNotice, bool) {
	return termprompt.MatchNotice(a.spec, screen)
}

// The concrete agent must satisfy the capability interface. A caller reaches this
// through a type assertion, which cannot fail at build time, so the guarantee it
// gives up is pinned here instead.
var _ NoticeMatcher = (*agent)(nil)
