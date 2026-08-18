// Package termwait detects the one thing a chat cannot otherwise report: its CLI
// is parked on a modal that reaches Crowbar through no hook, and only a human at
// the terminal can clear it.
//
// Crowbar answers the prompts that DO travel through hooks — tool permissions,
// questions, MCP elicitations — from the chat itself. This is the complement: the
// prompts it fundamentally cannot answer. There is no channel to reply on, so the
// only correct behaviour is to notice, say so, and get the user to the terminal.
//
// # The gates, and why the order is the design
//
// A chat is reported waiting only when ALL of these hold, checked in this order:
//
//  1. it has a LIVE RUNNER. No process, nothing to be blocked.
//  2. it is NOT Working. A busy agent is not stuck; it is busy, and the spinner
//     already says so.
//  3. it has NO PENDING CHOICE. A prompt that arrived over a hook is answerable
//     IN THE CHAT, and the chat is already showing its card. This gate is what
//     stops an ordinary permission prompt being relabelled "your agent is stuck
//     in the terminal" and marching the user somewhere they did not need to go.
//  4. its screen MATCHES a needle its provider declared.
//
// Getting that order wrong produces one of exactly two failures, and both are
// worse than the bug this fixes: a false "it's stuck" banner over a healthy idle
// chat, or a hijack of a prompt the chat could already answer.
//
// # Cost
//
// This runs on a cadence, so it is built to be nearly free when nothing is
// happening. Gates 1–3 are in-memory reads. Gate 4 — the only expensive one, since
// it renders a cell grid to text — is additionally skipped whenever the PTY's
// screen has not moved since this detector last looked, which for a chat parked on
// a dialog is EVERY tick after the first. See Detector.Evaluate.
package termwait

import (
	"context"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// DefaultInterval is how often the sweep re-asks the question.
//
// Chosen as a compromise between two costs that pull opposite ways. The user is
// staring at a pane that explains nothing, so latency is felt directly — and this
// codebase has a documented history of idle-CPU regressions from render and poll
// loops, so a hot loop is not on the table. Two seconds bounds the worst-case
// "the pane looked dead" window at about two seconds while leaving the steady
// state at a handful of map lookups and one integer compare per live chat: a
// parked chat emits no output, so its generation never moves and its screen is
// never re-rendered.
const DefaultInterval = 2 * time.Second

// Runners is the live-runner census. A row exists exactly while its PTY does, so
// this is both "which chats have a process" and "which PTY each one holds".
type Runners interface {
	AllLive(ctx context.Context) ([]domain.AgentRunner, error)
}

// Chats answers the busy question.
//
// Working is READ here, never re-derived. It is the chat aggregate's own fold —
// an open turn OR outstanding async work — and a second copy of that rule in this
// package could disagree with it, which is exactly the class of bug that made the
// spinner lie in the first place.
type Chats interface {
	GetChat(ctx context.Context, id string) (domain.AgentChat, error)
}

// Choices answers whether a prompt the chat can already handle is outstanding.
type Choices interface {
	PendingChoices(ctx context.Context, chatID string) ([]domain.ActivityChoice, error)
}

// Screens is the daemon's own read of a PTY's visible screen — the VT model it
// maintains for every session, not anything a browser scraped.
//
// `since` is a change generation: pass the value from the last read and an
// unmoved screen answers changed=false with no text, which is what makes a parked
// chat cost nothing to re-check.
type Screens interface {
	Screen(sessionID string, since uint64) (text string, gen uint64, changed bool)
}

// Prompts matches a screen against the needles a provider's descriptor declares.
// The needles are provider vocabulary and stay on the far side of this seam;
// nothing here learns a CLI's words.
type Prompts interface {
	MatchTerminalPrompt(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalPrompt, bool)
}

// Publish is how a changed verdict leaves the daemon. It is called only on a
// CHANGE, never per tick, so a chat that has been parked for an hour has produced
// exactly one call.
type Publish func(chatID, workspaceID string, wait domain.AgentTerminalWait)

// Deps is the detector's dependency set.
type Deps struct {
	Runners Runners
	Chats   Chats
	Choices Choices
	Screens Screens
	Prompts Prompts

	// Interval overrides DefaultInterval. Zero takes the default.
	Interval time.Duration
}

// Detector reports which chats are blocked behind a terminal-only prompt.
type Detector interface {
	// Wait returns the last computed verdict for a chat. It is a READ of the
	// sweep's answer, never a fresh evaluation: a REST read must not be able to
	// drive provider-screen work on the request path.
	Wait(chatID string) domain.AgentTerminalWait

	// Sweep re-evaluates every chat holding a live runner and publishes the ones
	// whose answer changed. Exported for the tests and for a caller that wants one
	// deterministic pass rather than a loop.
	Sweep(ctx context.Context, publish Publish)

	// Run drives Sweep on a cadence until ctx is done. It returns immediately;
	// the loop runs on its own goroutine.
	Run(ctx context.Context, publish Publish)
}

type detector struct {
	deps Deps

	mu sync.RWMutex
	// state is keyed by CHAT id, not runner id, because the verdict is published
	// about a chat and read back about a chat. A runner moving between chats
	// therefore leaves no verdict behind on the chat it left: the sweep sees that
	// chat is no longer in the live census and forgets it.
	state map[string]chatState
}

// chatState is one chat's published verdict plus the cache that lets the next tick
// avoid re-reading a screen that has not moved.
type chatState struct {
	// workspaceID is remembered because the feed is workspace-scoped and the chat
	// whose verdict must be CLEARED is, by definition, one whose runner has just
	// left the census — so there is nothing left to ask which workspace it was in.
	workspaceID string
	screen      screenCache
	// published is the last verdict handed to Publish, and what Wait returns.
	// Comparing against it is what makes the feed carry changes rather than ticks.
	published domain.AgentTerminalWait
}

// screenCache remembers what the SCREEN said, and the generation it said it at.
//
// The generation belongs to the SESSION, so it is stored beside the session id it
// came from: a chat whose runner was replaced gets a different PTY whose counter
// starts again, and carrying the old generation across would let a fresh screen
// read as unchanged.
//
// `matched` is deliberately separate from the published verdict, because the two
// change for different reasons. Gates 2 and 3 are re-read every tick and can flip
// with no terminal output whatsoever — a turn starts, a permission prompt opens —
// so the verdict must be recomputed from them even on a tick that found the screen
// exactly where it left it.
type screenCache struct {
	session string
	gen     uint64
	matched domain.AgentTerminalWait
}

// New constructs the detector.
func New(deps Deps) Detector {
	return &detector{deps: deps, state: make(map[string]chatState)}
}

func (d *detector) Wait(chatID string) domain.AgentTerminalWait {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.state[chatID].published
}

func (d *detector) interval() time.Duration {
	if d.deps.Interval > 0 {
		return d.deps.Interval
	}
	return DefaultInterval
}
