// Package termwait reads a chat's PTY screen once per sweep and answers the two
// questions no hook can answer, both of which end with a pane that explains
// nothing.
//
// # Question one: is the CLI parked on a modal only a human can clear?
//
// Crowbar answers the prompts that DO travel through hooks — tool permissions,
// questions, MCP elicitations — from the chat itself. This is the complement: the
// prompts it fundamentally cannot answer. There is no channel to reply on, so the
// only correct behaviour is to notice, say so, and get the user to the terminal.
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
// # Question two: did the CLI abandon a turn without ever saying so?
//
// The second question is the FIRST ONE'S MIRROR IMAGE, which is why it lives here
// rather than in a package of its own. A provider can take a turn, fail it, paint
// a sentence explaining why, and then neither finish the turn nor exit. Measured
// against codex-cli 0.146.0 out of quota: user_prompt fired, the turn opened, the
// usage-limit banner appeared, and then nothing — no Stop hook, no SessionEnd,
// and a LIVE process, so the runner-exit reconcile that closes an abandoned turn
// (agent.reconcileRunnerExit → closeAbandonedTurn) was never reached either. The
// turn stayed open for 44 minutes and closed only because a human switched
// provider. Nothing in the daemon would ever have closed it.
//
// A turn is reported STALLED only when ALL of these hold:
//
//  1. it has a LIVE RUNNER holding a PTY. A session with no screen — dead,
//     unknown, or a suspended placeholder, all of which answer gen == 0 — is NO
//     EVIDENCE, and no evidence closes nothing.
//  2. the chat IS Working. Gate 2 inverted: there has to be something to close.
//  3. its screen has NOT MOVED for a bounded quiet period (DefaultStallQuiet).
//     Any change at all resets the clock to zero.
//  4. the quiescent screen matches a provider notice declaring ends_turn.
//  5. it has NO PENDING CHOICE. A chat blocked on a prompt is waiting on the
//     human, and its Working is honest.
//  6. its conversation record shows NO OPEN TOOL CALL and NO OPEN SUBAGENT. This
//     is the one gate that answers "is it working?" from HOOK evidence rather
//     than from pixels, and it is what covers the provider whose painting
//     behaviour has not been measured — see below.
//
// THE ABSOLUTE CONSTRAINT IS THAT THE SPINNER NEVER GOES DARK WHILE THE AGENT IS
// GENUINELY WORKING, so no timer alone, ever: gates 3 and 4 are two independent
// signals that must agree. A timer alone would eventually close every long turn.
// A notice alone would close a turn the instant a banner appeared, which is wrong
// while output is still moving. Only their conjunction is evidence.
//
// # What "the screen has not moved" is measured against
//
// THE RENDERED TEXT, not the generation counter, and the distinction is the
// difference between this working and this silently never firing.
//
// The generation bumps on ANY consumed PTY chunk, including one that repaints
// byte-identical cells. A TUI that periodically redraws its own chrome — a tip
// line, a placeholder, a clock — would therefore look like movement, reset the
// clock every time, and, if its repaint period were shorter than stallQuiet,
// defer the close FOREVER. The wedge would be back with the fix installed and
// every test passing.
//
// So the generation is kept, but only as the cheap "might have changed" gate it
// is: an unmoved counter still skips the render, which is what keeps an idle
// daemon free. When it HAS moved, the freshly rendered text is compared against
// the text this detector last kept, and three properties follow:
//
//   - A REPAINT OF IDENTICAL CONTENT IS NOT MOVEMENT. The clock keeps running,
//     and the turn closes on exactly the deadline it would have had if nothing
//     had arrived at all.
//   - PURE CURSOR MOVEMENT IS NOT MOVEMENT EITHER, and that falls out by
//     construction rather than by intent: Screen renders the viewport as content
//     only — no SGR, no cursor, no scrollback (model.ScreenReader) — so a moved
//     cursor is not in the string being compared and cannot appear as a change.
//   - GENUINE ACTIVITY STILL RESETS IT. This is the asymmetry that must never be
//     optimised away. A working agent changes TEXT, not merely bytes: claude's
//     spinner carries its own elapsed-time counter (`· Simmering… (6s · thought
//     for 5s)` → `(7s · …)`), so every repaint of a live turn differs from the
//     last and the clock is pinned at zero for as long as it runs.
//
// # Why screen quiescence is a sound busy signal, and which way it errs
//
// Measured against claude 2.1.234 in a real PTY, counting the model writes that
// bump the session's screen generation. Idle, composer empty, sampled every 5s
// for 100s: the counter sat at 12 for all twenty samples — completely frozen.
// Mid-generation with prose visibly streaming, sampled every 2s for 48s: 34, 46,
// 59, 75, 87, … 178 — monotonically climbing, with no 2-second window anywhere in
// it that saw zero writes. So a busy CLI moves its generation continuously, and
// an idle one does not move it at all.
//
// Measured against codex-cli 0.146.0, idle at the composer past the hooks modal
// and sampled every 25s for 275s — well over the quiet window: writes=63 at all
// twelve samples, and twelve rendered snapshots with ONE distinct hash between
// them, tip line and placeholder included. codex does NOT rotate its chrome
// within a session; the placeholder rotation reported earlier is chosen once at
// process start and differs only BETWEEN fresh sessions, which cannot reset a
// within-session clock. The live harness saw the same in the wedged state:
// byte-identical for 2m15s, close at 143.6s.
//
// The text comparison is therefore not load-bearing for either CLI shipped
// today. It is here so the property does not depend on that staying true of the
// next release. The alternative — excluding known-volatile rows from the
// comparison — was rejected: it needs per-provider knowledge of which rows are
// chrome, it rots with every CLI release, and it risks excluding the very rows
// that prove an agent is alive.
//
// Nothing here depends on recognising a BUSY screen. Neither shipped CLI has a
// usable idle needle (codex's composer placeholder rotates between sessions;
// claude's footer hint is painted while it is generating and is mode- and
// model-dependent), and no codex busy indicator was ever captured — the account
// was rate-limited for the whole probe. See claude.yaml's trailing comment, which
// exists so nobody adds one later.
//
// # The gate that does not depend on pixels at all
//
// That quiescence argument is MEASURED FOR CLAUDE AND ASSUMED FOR CODEX. The
// codex account was rate-limited throughout, so no codex turn could be run and
// its painting behaviour while working is genuinely unknown. The hole that leaves
// is narrow but real: codex working, painting nothing for a whole quiet window,
// with a stale usage-limit banner still in the viewport from an earlier attempt —
// which would close a live turn, the one outcome that must never be produced.
//
// Gate 6 closes it with evidence that has nothing to do with the screen. An entry
// exists in the conversation record's open-state maps exactly between a
// provider's tool_pre and its tool_post, so a CLI that is mid-tool is PROVABLY
// working however frozen its screen looks. Its error runs the same way every
// other decision here does: a tool whose completion hook never arrives leaves
// that chat permanently ineligible for reconcile, so it stays wedged — annoying,
// self-evidently a bug elsewhere, and strictly better than a spinner going dark
// under a working agent. That asymmetry is what this whole feature is built on.
//
// # Cost
//
// This runs on a cadence, so it is built to be nearly free when nothing is
// happening. The gates that can be answered from memory are answered before the
// one that cannot, and the screen read — the only expensive gate, since it
// renders a cell grid to text — is skipped whenever the PTY has not moved since
// this detector last looked, which for a parked or idle chat is EVERY tick after
// the first.
//
// ONE read answers both questions. That is not an optimisation, it is the reason
// the second question lives in this package: a second detector on its own ticker
// would render the same screen a second time on every change, and this codebase's
// documented idle-CPU history does not permit that. The cost that IS new is that
// a WORKING chat's screen is now read too — it used to short-circuit at gate 2 —
// so a chat with output streaming renders once per interval while it streams.
// That is one render per live working chat, and it buys the only signal that can
// distinguish a working CLI from a wedged one.
//
// The two REPOSITORY gates — pending choices, and open tools/subagents — are
// deliberately LAST on the stall path rather than fifth and sixth as the gate
// list reads. Asking them every tick for every working chat in the daemon would
// be two database queries per chat per interval, forever, to answer a question
// that matters at most once in a chat's life. They are consulted only once the
// in-memory gates have already agreed, which for a healthy daemon is never.
// Correctness is unaffected: all six gates must hold, and conjunction does not
// care about order.
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

// DefaultStallQuiet is how long a working chat's screen must sit COMPLETELY STILL
// before a matching end-of-turn notice on it is believed.
//
// Two minutes, and the argument for it is that the risk is entirely one-sided.
// The failure this closes is PERMANENT — a wedged turn is not self-healing, and
// the measured one lasted 44 minutes and would have lasted forever — so waiting
// two minutes costs a user who is already stuck nothing they will notice. The
// opposite error, closing a turn on an agent that was merely quiet for a moment,
// darkens a working spinner and drops the chat's own record of what it was doing.
// So the number is not tuned for latency at all: it is set far above any pause a
// working CLI has ever been measured to take (the longest observed gap between
// consecutive screen writes on a generating claude was under two seconds), and
// then left there.
//
// It is a floor rather than an exact period: the clock starts when a sweep first
// OBSERVES the screen at rest, which is up to one interval after it actually came
// to rest, so the real quiet period is always at least this long.
const DefaultStallQuiet = 120 * time.Second

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
//
// gen == 0 means the engine cannot answer for this session at all. For the stall
// question that is treated as no evidence, and it is worth recording that for the
// case the stall question exists to fix it is unreachable: the terminal engine's
// maintenance sweep skips agentic CLI sessions in both of its phases (`if
// s.IsCommand() { continue }`), so a live agent runner is never suspended and
// always has a live VT model behind it. The guard stays because it is still the
// correct answer for a session that is genuinely dead or unknown.
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

// Notices is the same seam for the second question: the messages a provider
// paints INSTEAD of finishing a turn.
//
// It is a separate port from Prompts because a provider genuinely may declare one
// and not the other — claude declares prompts and no notices — and because a
// caller wiring only the first must keep working unchanged. A nil Notices means
// no chat is ever reported stalled, which is exactly the behaviour of a daemon
// built before this existed.
type Notices interface {
	MatchTerminalNotice(
		ctx context.Context,
		providerID string,
		screen string,
	) (engineagents.TerminalNotice, bool)
}

// Work answers, from the conversation record rather than from the screen,
// whether this chat still has something RUNNING: a tool call opened by a
// provider's tool_pre with no tool_post yet, or a subagent started and not
// stopped.
//
// It is a boolean question rather than a row read on purpose. What is open is the
// activity aggregate's business, and a detector that learned the shape of a tool
// row would be a second place that has to be updated when one changes.
type Work interface {
	OpenWork(ctx context.Context, chatID string) (bool, error)
}

// Publish is how a changed WAIT verdict leaves the daemon. It is called only on a
// CHANGE, never per tick, so a chat that has been parked for an hour has produced
// exactly one call.
type Publish func(chatID, workspaceID string, wait domain.AgentTerminalWait)

// Stall is one chat whose turn the CLI abandoned without saying so.
//
// It carries the runner's identity as well as the chat's because the notice is
// recorded AS A TURN in that chat's conversation, and a turn that named no
// provider, runner or session would be the one row in the record nobody could
// trace back to the process that produced it.
type Stall struct {
	ChatID      string
	WorkspaceID string
	ProviderID  string
	RunnerID    string
	SessionID   string

	// Notice is the provider's own sentence, captured off the screen. It is the
	// reason the turn is being closed and the only thing in this struct a user
	// ever sees.
	Notice engineagents.TerminalNotice
}

// Stalled is how a stall verdict leaves the detector: a callback into the
// usecase, which owns closing the turn and recording the notice.
//
// It is on Deps rather than a Sweep parameter — unlike Publish, which is the hub
// and is wired a layer above — because the closer IS the usecase that constructs
// this detector, and it exists before the detector does.
//
// It is called at most ONCE per stall: the verdict is latched against the screen
// generation that produced it, so a chat whose Working flag has not yet caught up
// with the close is not closed again on the next tick.
type Stalled func(ctx context.Context, stall Stall)

// Deps is the detector's dependency set.
type Deps struct {
	Runners Runners
	Chats   Chats
	Choices Choices
	Screens Screens
	Prompts Prompts

	// Notices, Work and OnStall are the second question, and ALL THREE are
	// required for it to be asked. Any one of them nil means no turn is ever
	// closed here — which is both the behaviour of a daemon built before this
	// existed and the safe direction to fail in, since the alternative to
	// "cannot ask" is never "assume the answer is yes".
	Notices Notices
	Work    Work
	OnStall Stalled

	// Interval overrides DefaultInterval. Zero takes the default.
	Interval time.Duration

	// StallQuiet overrides DefaultStallQuiet. Zero takes the default.
	StallQuiet time.Duration

	// Now is the clock the quiet period is measured on. Zero takes time.Now.
	//
	// Injectable because the alternative in a test is a sleep, and a test that
	// sleeps to prove a 120-second rule is a test that either takes two minutes
	// or proves nothing. Every timing assertion in this package's tests is made
	// by moving this clock and calling Sweep, so none of them can be flaky.
	Now func() time.Time
}

// Detector reports which chats are blocked behind a terminal-only prompt, and
// which have had a turn abandoned under them.
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

// screenCache remembers what the SCREEN said, the generation it said it at, and
// how long it has been saying it.
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

	// text is the last rendered screen, kept VERBATIM rather than hashed.
	//
	// It is what the quiet clock is actually measured against: the generation
	// counter says only that BYTES ARRIVED, and a TUI that repaints byte-identical
	// cells bumps it without the screen having changed at all. See readScreen.
	//
	// A 64-bit hash would be cheaper to hold and was the obvious alternative. It
	// is not used because its failure mode points the wrong way: a collision reads
	// as "the screen did not change" while it did, which keeps a clock running
	// that should have reset — the one error this whole feature refuses to make.
	// The cost of exactness is one rendered viewport per LIVE chat, a few KB
	// against the full VT model the terminal engine already holds for that same
	// session, so there is nothing here worth trading a one-sided risk for.
	text string

	matched domain.AgentTerminalWait

	// notice is what the same read said about the second question. It is cached
	// alongside `matched` for the same reason and by the same rule: it describes
	// THIS generation, and any change at all replaces it — including replacing it
	// with nothing, which is how a banner scrolling off the screen withdraws the
	// evidence for closing a turn.
	notice engineagents.TerminalNotice

	// since is when this generation was first OBSERVED at rest, and it is the
	// whole of the quiet clock. It is reset by any change, so it measures
	// stillness and never age.
	since time.Time

	// fired latches a stall already acted on, so one wedge produces one closed
	// turn and one notice.
	//
	// It has to be here, tied to the generation, because the close is not
	// instantly visible: Working is read from an asynchronously folded projection,
	// so the next tick or two can still see the turn open. Without the latch that
	// would append a second notice to the chat. It clears with the cache whenever
	// the screen moves — a genuinely new screen is a genuinely new question.
	fired bool
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

func (d *detector) stallQuiet() time.Duration {
	if d.deps.StallQuiet > 0 {
		return d.deps.StallQuiet
	}
	return DefaultStallQuiet
}

func (d *detector) now() time.Time {
	if d.deps.Now != nil {
		return d.deps.Now()
	}
	return time.Now()
}
