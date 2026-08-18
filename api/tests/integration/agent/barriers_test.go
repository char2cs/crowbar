//go:build integration

package agent_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// This file holds the harness's SYNCHRONISATION seams. There are exactly two
// things a test driving a real vendor CLI ever has to wait for, and neither of
// them is an amount of time:
//
//   - The CLI told the daemon something. Every state this suite asserts on —
//     a conversation bound, a runner moved by /clear, a turn appended to the
//     ledger — is produced by the CLI shelling out to `crowbar hook`, which POSTs
//     to the daemon. That POST is an EVENT, and hookBarrier turns it into one a
//     test can block on (awaitHook).
//
//   - The CLI is showing us something. A modal to acknowledge, or the text we typed
//     echoed back into its composer. Both are visible ON SCREEN, and kit.PTYTap turns
//     the PTY into a signal (settleCLI, drive).
//
// The suite used to sleep for both, because it had neither seam: nudgeUntil
// re-sampled the read model every 250ms while blindly firing Enter into the PTY
// every 2s, and a bare time.Sleep(300ms) stood in for "the TUI has taken my
// input". Those are guesses about how fast this machine is, and they are wrong
// under load — which is exactly when the CI runs.

// backstop bounds a wait so a failure is DIAGNOSED rather than left to hang until
// `go test -timeout` kills the binary with no message. It is NOT a synchronisation
// device: no barrier below ever waits for it on the happy path — each one returns
// the instant its real signal arrives — and lengthening it can never make a passing
// test pass "more" or a slow machine pass "differently".
//
// It is deliberately far larger than any legitimate wait. The slowest real thing
// here is one model turn on a resumed codex, which must first finish booting the
// MCP servers the user's own ~/.codex config declares (observed live at ~50s before
// the turn is even dispatched) and then think. A bound has to clear that with room
// to spare, or it stops being a bound on FAILURE and starts being a bound on the
// vendor's mood — which is the very thing this rewrite exists to remove.
const backstop = 5 * time.Minute

// hookBarrier observes COMPLETED agent-hook requests: gin middleware fires it
// after the handler chain for POST .../agent/hooks has returned.
//
// "After" is the whole point, and it is why this is middleware rather than a hub
// subscription. When the handler returns, IngestHook has already run to
// completion on that goroutine: the reducer's outcome is committed (runner
// commands are asynx SendWait) and any ledger turn is physically written to disk
// — appendTurn is a synchronous file write inside IngestHook. So a test woken by
// this edge can immediately READ the thing the hook produced, with no second race
// to lose. A hub event would be the wrong seam: the hub is broadcast from inside
// the fold, so a turn_stopped frame is emitted BEFORE the ledger append it
// announces, and a test that read the ledger on it would race the write.
type hookBarrier struct {
	sig *kit.Signal
}

func newHookBarrier() *hookBarrier {
	return &hookBarrier{sig: kit.NewSignal()}
}

// middleware fires the barrier once the hook handler chain has fully returned.
func (b *hookBarrier) middleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()
		if c.Request.Method == "POST" && strings.HasSuffix(c.FullPath(), "/agent/hooks") {
			b.sig.Fire()
		}
	}
}

// awaitHook blocks until pred holds, re-evaluating it every time a vendor CLI's
// hook lands. It is the replacement for nudgeUntil, and it needs no nudging: every
// modal is acknowledged up front, deterministically, by settleCLI, so the CLI is
// never sitting behind one that a stray Enter has to clear.
//
// Every predicate the suite uses is advanced by a hook and nothing else, so the
// hook edge is a complete wakeup source: SessionStart binds a conversation and
// moves a runner; UserPromptSubmit and Stop append ledger turns.
//
// Each edge is followed by a Quiesce so the async read-model projections a hook's
// commands kick off have folded before pred looks at them. The commands
// themselves are SendWait, so the aggregate is already durable when the handler
// returns; Quiesce closes the gap for the independent store/list projections that
// trail it.
func awaitHook[T any](
	t *testing.T,
	h *harness,
	what string,
	pred func() (T, bool),
) T {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()
	return kit.Await(t, ctx, what, func() (T, bool) {
		h.app.Repositories.WaitQuiescent()
		return pred()
	}, h.hooks.sig)
}

// trustNeedle is the string each vendor CLI paints when it is BLOCKING on its
// first-run "do you trust this folder?" dialog — captured from the real CLIs
// (claude 2.1.207, codex 0.139.0) rather than guessed:
//
//	claude: "❯ 1. Yes, I trust this folder" / "Enter to confirm · Esc to cancel"
//	codex:  "› 1. Yes, continue"            / "Press enter to continue"
var trustNeedle = map[string]string{
	"claude": "Enter to confirm",
	"codex":  "Press enter to continue",
}

// firstOfProvider reports whether this is the FIRST CLI of the given provider in
// this harness — i.e. the one that will be shown a trust dialog — and records it.
//
// A vendor CLI asks about trust ONCE per place, and remembers the answer in ITS OWN
// home, keyed by a path: claude by the CWD, codex by the REPOSITORY ROOT. Those homes
// are the user's real ones in production and throwaway ones here — newHarness calls
// kit.IsolateProviderHomes, so the answers this suite produces are written to a temp
// directory and thrown away with it, instead of accumulating one dead stanza per test
// run in the user's own ~/.codex/config.toml and ~/.claude.json forever.
//
// (Isolating claude was long believed impossible because CLAUDE_CONFIG_DIR "breaks its
// auth". It does not break it so much as RENAME it: claude derives its keychain service
// name from the config dir, so moving the dir makes it look up an item that does not
// exist. CLAUDE_SECURESTORAGE_CONFIG_DIR="" pins the lookup back to the real
// credential. See kit.IsolateProviderHomes.)
//
// What matters HERE is only that isolation does not change the SHAPE of the first run:
// a throwaway home trusts nothing, exactly like a never-before-seen repo path in a real
// home, so the dialog still appears exactly once per provider per test. Every test here
// builds ONE fresh temp repo with ONE worktree, so within a single test:
//
//   - the first claude/codex spawned sees a brand-new path → it ALWAYS shows the
//     dialog, and blocking on it is safe;
//   - every later CLI of that same provider — the incoming half of a provider
//     switch, a switch-back — is in a place its provider already trusts → it shows
//     NOTHING, and blocking on a dialog would hang until the backstop.
//
// Both halves are verified live, not assumed: a second codex switched back into the
// same repo paints its composer with no dialog at all, and a second claude resumed
// into the same worktree likewise.
//
// This is knowledge the HARNESS has and the screen does not, which is why it is
// tracked here rather than scraped: "no dialog has appeared yet" and "no dialog is
// ever coming" look identical on a PTY, and only one of them is safe to type into.
func (h *harness) firstOfProvider(
	provider string,
) bool {
	if h.trusted[provider] {
		return false
	}
	h.trusted[provider] = true
	return true
}

// resumeNeedle is what a RESUMED codex paints, and it is a modal state in all but
// name: SwitchProvider terminates the outgoing CLI, so the codex that comes back
// re-opens a conversation that was cut off mid-flight and greets the user with
//
//	"■ Conversation interrupted - tell the model what to do differently."
//
// Until that is acknowledged with a keypress, codex ACCEPTS TYPED TEXT BUT WILL NOT
// SUBMIT IT: the prompt lands in the composer, echoes back, and then Enter does
// nothing at all — verified live, with the PTY emitting not one further byte across
// 45s and fifteen further Enters. It is the same shape of trap as the trust dialog,
// and it is invisible unless you look for it.
//
// (This is exactly what the old 30-second "settle" of blind Enters was really for.
// It cleared this banner by accident, before any text was typed, and nobody had to
// know it existed. Sending the Enter when the banner is actually ON SCREEN does the
// same job deliberately, in milliseconds, and says why.)
const resumeNeedle = "Conversation interrupted"

// claudeReadyNeedle is claude's interactive FOOTER, and waiting for it is what stops a
// resumed claude from silently eating the first thing typed into it.
//
// A resumed claude paints no modal, and this harness used to conclude from that that it
// needed no barrier at all: spawn it, type. That is wrong, and it is wrong in the one way
// a PTY cannot show you. The prompt is not merely typed EARLY — it is DISCARDED. Ink
// drains whatever is already sitting in the tty at the moment it takes the keyboard, so
// anything written into the gap between spawn and mount is swallowed whole. What is left
// on screen is a fully booted claude with an EMPTY composer, which is indistinguishable
// from one that is simply thinking; drive() then waits out its whole backstop for an echo
// that can never come. (Every switch-back test failed exactly this way, at 306 seconds.)
//
// BINDING IS NOT THE SIGNAL, and that is the trap. claude's SessionStart hook fires at
// BOOT — before the composer exists — so the runner is bound, the conversation id is
// right, every assertion about the model passes, and the CLI still is not listening. The
// switch-back tests proved this the expensive way: they all bound, and all lost the prompt.
//
// The footer is a signal and not a guess because it is part of the interactive UI itself:
// it cannot be on screen unless that UI has mounted, and the UI does not mount without
// taking the keyboard. "(shift+tab to cycle)" is the stable part — the mode NAME beside it
// changes with the user's config ("auto mode on", "accept edits on"), the hint does not.
//
// The fresh-claude path needs none of this and does not get it: a claude sitting on its
// trust dialog has ALREADY mounted (the dialog IS the UI), so its keyboard is live and the
// Enter that answers the dialog is read, not drained.
const claudeReadyNeedle = "shift+tab to cycle"

// settleCLI brings a freshly started vendor CLI to a state where a typed prompt will
// actually be SUBMITTED, blocking on what the CLI itself paints rather than on a clock.
//
// Which modal it must clear depends on whether this is the provider's first CLI in
// this test (see firstOfProvider):
//
//   - FIRST: the folder is new to the provider, so it asks for trust and — for claude —
//     fires no hook whatsoever until answered.
//   - LATER: the folder is already trusted, so no trust dialog is coming. A codex here
//     is by definition a RESUMED codex, and shows the "Conversation interrupted" banner
//     that silently swallows submits until acknowledged. A resumed claude shows neither,
//     and needs nothing.
//
// Every arm fails FAST if the PTY dies, printing the screen the CLI died on — which is
// how a broken `--resume` surfaces as a diagnosis instead of an unexplained timeout.
func settleCLI(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
	provider string,
	runnerID string,
) {
	t.Helper()

	needle := ""
	switch {
	case h.firstOfProvider(provider):
		needle = trustNeedle[provider]
	case provider == "codex":
		needle = resumeNeedle
	default:
		// A resumed claude. No modal is coming — but it is NOT ready to be typed into the
		// instant it is spawned, and the input it drops on the way up is gone for good.
		// Wait for its composer, and do it HERE rather than folding it into the modal
		// machinery below: that arm may finish on "the runner is already bound", which is
		// a sound proof of being past a MODAL and no proof at all of being ready for a
		// KEYSTROKE (claude binds at boot, before the UI exists). See claudeReadyNeedle.
		awaitComposer(t, h, tap, termSessID, provider, "while coming back up on --resume")
		return
	}
	if needle == "" {
		t.Fatalf("settleCLI: no needle known for provider %q", provider)
	}

	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()

	const (
		sawNeedle = "needle"
		sawBound  = "already-past"
	)
	outcome := kit.Await(t, ctx, provider+" to paint "+needle,
		func() (string, bool) {
			if tap.Contains(needle) {
				return sawNeedle, true
			}
			// Already bound == already past every modal: only a running CLI can have fired
			// the SessionStart hook that binds it.
			if r, err := h.app.Repositories.AgentRunner.Get(context.Background(), runnerID); err == nil &&
				r.CurrentSession != "" {
				return sawBound, true
			}
			requireCLIAlive(t, h, tap, termSessID, provider, "while starting up")
			return "", false
		}, tap.Signal(), h.hooks.sig)

	if outcome == sawNeedle {
		if err := h.eng.Terminal.Write(context.Background(), termSessID, []byte("\r")); err != nil {
			t.Fatalf("settleCLI: acknowledge %s's %q: %v", provider, needle, err)
		}
	}
	if provider != "claude" {
		return
	}
	// ANSWERING claude's trust dialog is not the same as being ready for a keystroke,
	// and this is the second place the claudeReadyNeedle trap bites. Acknowledging the
	// dialog makes claude REMOUNT — it tears down the dialog and brings up the REPL —
	// and Ink drains whatever is already in the tty when the new UI takes the keyboard.
	// A prompt written into that gap is DISCARDED, leaving a fully booted claude with an
	// EMPTY composer, which looks exactly like one that is thinking; drive() then waits
	// out its whole backstop for an echo that can never come.
	//
	// Verified against 2.1.220 on a bare pty, both ways round: typing straight after the
	// acknowledging Enter never echoes (claude does not even repaint), while waiting for
	// the footer first echoes the full prompt. Only claude is affected — a fresh codex
	// answered the same way echoes in 0.02s, which is why this is provider-scoped rather
	// than applied to every CLI.
	//
	// No test hit this before because every existing claude driver crosses
	// awaitSessionBound first, and that wait happens to outlast the remount.
	awaitComposer(t, h, tap, termSessID, provider, "while mounting its composer")
}

// awaitComposer blocks until claude has painted its interactive FOOTER — the proof
// that its REPL has mounted and therefore has the keyboard. See claudeReadyNeedle
// for why nothing weaker will do, and settleCLI for the two states that need it.
//
// It is CLAUDE-ONLY, and it says so out loud rather than trusting its caller. The
// needle it waits for is claude's footer and nothing else, so handing it any other
// provider would wait for text that CLI never paints — a hang until the backstop,
// reported as "the CLI never became ready", which is the most misleading failure this
// suite can produce. A future codex caller needs codex's own ready needle, not this
// function, and this is where that has to be noticed.
func awaitComposer(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
	provider string,
	when string,
) {
	t.Helper()
	if provider != "claude" {
		t.Fatalf("awaitComposer: only claude's composer needle (%q) is known; %q needs its own "+
			"ready needle rather than this wait, which would hang until the backstop",
			claudeReadyNeedle, provider)
	}
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()
	kit.Await(t, ctx, provider+" to paint its composer (ready for input)",
		func() (bool, bool) {
			if tap.Contains(claudeReadyNeedle) {
				return true, true
			}
			requireCLIAlive(t, h, tap, termSessID, provider, when)
			return false, false
		}, tap.Signal())
}

// requireCLIAlive fails the test — immediately, and with the screen the CLI died on —
// if its PTY is gone. A dead CLI can never satisfy any barrier, so every wait checks
// this rather than letting the backstop turn a hard failure into a mystery timeout.
func requireCLIAlive(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
	provider string,
	when string,
) {
	t.Helper()
	if h.eng.Terminal.SessionLive(context.Background(), termSessID) {
		return
	}
	t.Fatalf("the %s CLI EXITED %s. It did not run slowly — it DIED, and no barrier it was "+
		"being waited on could ever have been satisfied. Its screen was:\n%s",
		provider, when, kit.Readable(tap.Screen()))
}

// spawnReady spawns a provider on wsID, taps its PTY, dismisses its trust dialog,
// and returns the chat, the runner, the PTY session and the tap. It is the
// entry-point every test in this suite shares: a CLI that is past its modal and
// able to talk to the daemon.
//
// It deliberately does NOT wait for the CLI to bind a conversation. claude binds
// at boot (its SessionStart fires the moment the folder is trusted), but CODEX
// CREATES ITS ROLLOUT LAZILY, on its first real turn — an idle codex fires no
// SessionStart no matter how long it is watched. So "ready" here means "ready to
// be driven", and a caller that needs a conversation id must drive a turn first
// and await the bind after.
func spawnReady(
	t *testing.T,
	h *harness,
	wsID string,
	provider string,
) (chatID, runnerID, termSessID string, tap *kit.PTYTap) {
	t.Helper()
	ctx := context.Background()

	chatID, runnerID, err := h.app.Usecases.Agent.SpawnChat(ctx, wsID, provider)
	if err != nil {
		t.Fatalf("spawnReady: SpawnChat(%s): %v", provider, err)
	}
	termSessID = liveRunnerTerminalSession(t, h, chatID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })

	tap = kit.AttachPTY(t, h.eng.Terminal, termSessID)
	settleCLI(t, h, tap, termSessID, provider, runnerID)
	return chatID, runnerID, termSessID, tap
}

// attachReady is spawnReady's other half, for a CLI the DAEMON spawned rather
// than the test — the incoming process of a provider switch. It taps the new PTY
// and settles its trust dialog, so the switched-to CLI is drivable on the same
// terms as a directly spawned one.
func attachReady(
	t *testing.T,
	h *harness,
	termSessID string,
	provider string,
	runnerID string,
) *kit.PTYTap {
	t.Helper()
	tap := kit.AttachPTY(t, h.eng.Terminal, termSessID)
	settleCLI(t, h, tap, termSessID, provider, runnerID)
	return tap
}

// drive types a prompt into a live CLI's composer and submits it, blocking on the
// app's OWN ECHO in between — never on a clock.
//
// The two writes cannot be one. A trailing \r sent in the same write as the text is
// read by an Ink-style TUI as a literal newline INSIDE the input box, not as a
// submit, so the prompt is never sent; the text has to land first. What the old code
// did was sleep 300ms and hope the CLI had drained its stdin by then — a guess that
// is wrong exactly when the machine is loaded.
//
// The real signal is that the app has PAINTED the text back into its composer: that
// is the app telling us, on its own initiative, that it consumed the input. Matching
// is restricted to output painted after the write began (Mark), so a prompt typed
// twice in one session cannot match its own earlier echo. Only then is the
// submitting Enter sent.
//
// The caller must have run settleCLI first: a CLI sitting behind a modal will echo
// text into its composer and then silently refuse to submit it.
func drive(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	termSessID string,
	text string,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()

	mark := tap.Mark()
	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte(text)), "drive: write text")

	needle := kit.EchoNeedle(text)
	kit.Await(t, ctx, "the CLI to echo the typed prompt back into its composer", func() (bool, bool) {
		if kit.ScreenContains(tap.ScreenSince(mark), needle) {
			return true, true
		}
		requireCLIAlive(t, h, tap, termSessID, "driven", "while the prompt was being typed into it")
		return false, false
	}, tap.Signal())

	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte("\r")), "drive: submit")
}

// awaitSessionBound blocks until runnerID has announced a native conversation —
// the reducer's "bound" outcome, landed via the CLI's SessionStart hook — and
// returns it with the runner row it read.
//
// It is keyed on the RUNNER because a runner id is minted at spawn and is stable
// for the whole life of the process, INCLUDING across every conversation move it
// makes: this one lookup keeps answering after a /clear has carried the CLI into
// a different chat entirely. A dead runner reads as ErrNotFound rather than a
// stale row, so a CLI that died on startup surfaces as "never bound" instead of
// silently passing on a corpse.
// A dead CLI can never bind, so awaitSessionBound watches the PTY as well as the
// hook feed and fails the moment the process is gone — printing the screen it died
// on. Without that, a CLI that exits on startup (a `--resume` against a session the
// vendor no longer has, say) is indistinguishable from a slow one: the old code
// simply waited out its 30s timeout and reported "never bound", which reads like a
// flake and hides the actual error message the CLI printed.
func awaitSessionBound(
	t *testing.T,
	h *harness,
	runnerID string,
	termSessID string,
	tap *kit.PTYTap,
) (string, domain.AgentRunner) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()

	var last domain.AgentRunner
	sid := kit.Await(t, ctx, "runner "+runnerID+" to bind a provider conversation", func() (string, bool) {
		h.app.Repositories.WaitQuiescent()
		if runner, err := h.app.Repositories.AgentRunner.Get(context.Background(), runnerID); err == nil {
			last = runner
			if runner.CurrentSession != "" {
				return runner.CurrentSession, true
			}
		}
		// The PTY id is passed in rather than read off the runner row, because the row
		// is exactly what disappears when the CLI dies: its exit callback deletes it. A
		// liveness check keyed on the row could therefore never fire in the one case it
		// exists for.
		if !h.eng.Terminal.SessionLive(context.Background(), termSessID) {
			t.Fatalf("awaitSessionBound: the CLI on runner %s EXITED without ever binding a conversation. "+
				"It did not run slowly — it DIED. Its screen was:\n%s",
				runnerID, kit.Readable(tap.Screen()))
		}
		return "", false
	}, h.hooks.sig, tap.Signal())
	return sid, last
}

// awaitAssistantReply blocks until the ledger holds an assistant turn from
// provider whose text contains want, and returns every assistant turn seen.
//
// The signal is the CLI's own Stop hook: it POSTs, IngestHook writes the turn to
// disk, and the handler returns — so by the time this predicate re-runs, the
// .turn file is there to be read. That ordering is why the barrier is the
// completed HTTP request and not the hub's turn_stopped frame, which is broadcast
// from inside the fold, BEFORE the append it announces.
func awaitAssistantReply(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
	want string,
) []string {
	t.Helper()
	var texts []string
	awaitHook(t, h, provider+" to append an assistant turn containing "+want, func() (bool, bool) {
		texts = assistantReplies(readLedgerTurns(t, h, wsID, chatID), provider)
		for _, tx := range texts {
			if strings.Contains(tx, want) {
				return true, true
			}
		}
		return false, false
	})
	return texts
}

// awaitHandoffContains blocks until the chat's assembled handoff blob carries want.
//
// NOTE this is a weak barrier ON ITS OWN and must not be used to decide that a TURN
// has finished: the blob is rendered from every ledger entry, and the CLI's
// UserPromptSubmit hook records the prompt we typed the instant it is submitted — so
// a blob check for a codeword we ourselves typed is satisfied while the model is
// still thinking. Use awaitTurnComplete to wait for a turn.
func awaitHandoffContains(
	t *testing.T,
	h *harness,
	chatID string,
	want string,
) string {
	t.Helper()
	return awaitHook(t, h, "the ledger to carry "+want, func() (string, bool) {
		blob, err := h.app.Usecases.Agent.AssembleHandoff(context.Background(), chatID)
		if err != nil {
			return "", false
		}
		return blob, strings.Contains(blob, want)
	})
}

// awaitTurnComplete blocks until provider has appended an ASSISTANT turn to the
// chat's ledger — its own turn_stop hook having fired, on disk. That is the ONLY
// signal that a driven turn is actually over, and it is the barrier every provider
// SWITCH must cross first.
//
// Switching on anything weaker terminates the outgoing CLI mid-answer. That is not
// hypothetical: waiting instead on "the ledger mentions the codeword" — which the
// UserPromptSubmit hook satisfies the moment the prompt is submitted — killed codex
// while it was still composing its reply, so the reply never reached the ledger, its
// rollout never recorded it, and the codex resumed later had nothing to recall. The
// test that failed was the one asserting the resumed codex remembers; the bug was
// four steps earlier, in what "the turn is done" was taken to mean.
func awaitTurnComplete(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
) {
	t.Helper()
	awaitHook(t, h, provider+" to finish its turn (an assistant entry in the ledger)", func() (bool, bool) {
		ok := len(assistantReplies(readLedgerTurns(t, h, wsID, chatID), provider)) > 0
		return ok, ok
	})
}
