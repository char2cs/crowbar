package agent

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agent/internal/termwait"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// TerminalWait reports whether a chat's CLI is parked on a modal that reaches
// Crowbar through no hook — the workspace-trust dialog and its relatives — and
// which Crowbar therefore has no channel to answer.
//
// It is a READ of the detector's standing answer, never a fresh evaluation. A REST
// read must not be able to drive provider-screen work on the request path, and the
// answer it returns is at most one sweep old.
//
// The zero value is the ordinary answer, and it is also what a daemon built
// without a detector returns forever — which is what keeps every existing test
// harness and every provider declaring no needles behaving exactly as before.
func (u *Usecase) TerminalWait(chatID string) domain.AgentTerminalWait {
	if u.termWait == nil {
		return domain.AgentTerminalWait{}
	}
	return u.termWait.Wait(chatID)
}

// StartTerminalWaitSweep begins the cadence that keeps TerminalWait current,
// publishing each changed verdict through publish. It returns immediately.
//
// Separate from construction because the publisher is the hub, which is wired a
// layer above this usecase: the detector has to exist before anything can read
// through it, and the fan-out only exists once the API container is up.
func (u *Usecase) StartTerminalWaitSweep(
	ctx context.Context,
	publish func(chatID, workspaceID string, wait domain.AgentTerminalWait),
	promptSettled func(chatID, workspaceID, requestID string),
) {
	if u.termWait == nil {
		return
	}
	u.promptSettled = promptSettled
	u.termWait.Run(ctx, publish)
}

// MatchTerminalPrompt is the detector's provider seam: it resolves the descriptor
// for a provider and asks the engine whether this screen is one the CLI paints
// while blocked.
//
// The needles live in the descriptor, so a CLI release that repaints its dialog is
// a YAML edit on disk rather than a daemon build — and nothing in this package, or
// above it, learns a provider's vocabulary.
//
// home is the app-config Crowbar home rather than a workspace's, matching how
// every other provider-catalogue read resolves overrides (see ResolveProviders): a
// descriptor override is machine-level, not per-workspace. A failure to resolve is
// silence, not an error: an unresolvable descriptor declares no needles, and a
// provider that declares none never reports waiting.
func (u *Usecase) MatchTerminalPrompt(
	ctx context.Context,
	providerID string,
	screen string,
) (engineagents.TerminalPrompt, bool) {
	home, err := u.home()
	if err != nil {
		return engineagents.TerminalPrompt{}, false
	}
	descriptor, err := u.agents.Get(ctx, home, providerID)
	if err != nil {
		return engineagents.TerminalPrompt{}, false
	}
	return descriptor.MatchTerminalPrompt(screen)
}

// newTerminalWaitDetector builds the detector over the usecase's own ports. It is
// called from New once the usecase value exists, because two of the detector's
// dependencies — the descriptor lookup and the screen read — are the usecase's own
// seams rather than anything the caller passes in.
func newTerminalWaitDetector(u *Usecase) termwait.Detector {
	screens, ok := u.term.(termwait.Screens)
	if !ok {
		// A terminal seam that cannot render a screen (every test double that
		// does not need one) simply has no detector. Every chat then reports the
		// zero verdict forever, which is exactly the pre-existing behaviour.
		return nil
	}
	return termwait.New(termwait.Deps{
		Runners: u.runners,
		Chats:   u.chats,
		Choices: u.activity,
		Screens: screens,
		Prompts: u,
		// The stall half. Notices and Work are reads the usecase owns; OnStall is
		// the write, and it is wired here rather than passed in from the container
		// because — unlike the wait feed, which fans out through the hub — closing
		// a turn is this usecase's own business and nothing above it participates.
		Notices: u,
		Work:    u,
		OnStall: u.closeStalledTurn,
		// The third question. Same shape as the stall half: the read and the write
		// are both this usecase's own business, so neither is passed in.
		Deliveries: u,
	})
}
