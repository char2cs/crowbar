package terminal

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Deterministic shell synchronisation — no clocks.
//
// These tests drive REAL PTYs and REAL shells, so almost every wait in this package is
// some flavour of "has the shell got there yet?". A duration is not an answer to that
// question: it is a guess about how fast a fork/exec/prompt is on the machine that happens
// to be running CI today, and when the guess is wrong the test fails for reasons that have
// nothing to do with the code under test.
//
// A shell already has a perfectly good protocol for announcing where it is — it prints a
// PROMPT when it has finished everything it was asked to do and is blocked reading stdin.
// The helpers here make that protocol usable as a test signal.
//
// They live in a `package terminal` test file, and are exported, so that BOTH test packages
// share one implementation: the internal (package terminal) hardening/cover tests, and the
// external (package terminal_test) suite, which reaches them through thin aliases in
// shellsync_test.go.
//
// None of them takes a timeout. A wait that never completes is a HANG, and `go test
// -timeout` is the backstop that reports it — with a goroutine dump, which is strictly more
// diagnostic than a bespoke "did not happen within 5s" message would be.
// ---------------------------------------------------------------------------

// ShellPromptForTest is the PS1 handed to every pinned test shell. It is deliberately
// unlikely to occur in command output, so finding it on screen unambiguously means "the
// shell printed its prompt".
const ShellPromptForTest = "<<CRWB-RDY>>"

// PinShellForTest makes every session spawned by this test use a shell whose behaviour is a
// protocol rather than a lottery.
//
// Without it, sessions inherit the DEVELOPER'S $SHELL and their dotfiles — on the machine
// this was written on, /bin/zsh. A dotfile shell may print anything at any time, and may
// fork children of its own ASYNCHRONOUSLY (instant prompts, async git status, compinit). It
// therefore has no stable "idle" state to observe: the foreground process group flaps as its
// background work comes and goes, long after it first looked ready.
//
// That is not a hypothetical. It was the direct cause of an intermittent HANG in
// TestMaintenance_Phase3aIdleSuspend: the sweep's idle gate would see a transient zsh child
// and decline to suspend, the test's retry loop would spin runMaintenanceOnce as fast as the
// CPU allowed, and the spin would starve the very child it was waiting to finish. A livelock
// that no deadline could have fixed, because the test was racing a shell it did not control.
//
// Pinning /bin/sh with an explicit PS1 and no rc file removes the nondeterminism at its
// source rather than trying to out-wait it. The engine builds a session's environment from
// os.Environ() (see ptyEnv), so t.Setenv is what reaches the child shell; it also restores
// the previous values at test end.
func PinShellForTest(t *testing.T) {
	t.Helper()
	t.Setenv("SHELL", "/bin/sh")        // profile.Resolve's fallback for a nil profile
	t.Setenv("PS1", ShellPromptForTest) // the readiness protocol
	t.Setenv("ENV", "")                 // POSIX sh rc file: none, so no dotfile may speak
	t.Setenv("BASH_ENV", "")            // /bin/sh is bash-in-sh-mode on darwin; silence it too
}

// StripANSIForTest removes escape sequences from serializer output, leaving the screen's
// visible text. The serializer emits a clean, ground-state redraw (CSI cursor/SGR, OSC
// title, then literal cell text), so dropping the sequences yields the text a user would
// see — which is what the readiness predicates match on.
func StripANSIForTest(b []byte) string {
	var out strings.Builder
	for i := 0; i < len(b); {
		c := b[i]
		if c != 0x1b {
			// Keep printable text and newlines; drop other control bytes (notably \r, which
			// the serializer uses for positioning and which would otherwise glue rows).
			if c == '\n' || c >= 0x20 {
				out.WriteByte(c)
			}
			i++
			continue
		}
		i++ // consume ESC
		if i >= len(b) {
			break
		}
		switch b[i] {
		case '[': // CSI: params, then a final byte in @-~
			i++
			for i < len(b) && (b[i] < '@' || b[i] > '~') {
				i++
			}
			i++ // consume the final byte
		case ']', 'P', 'X', '^', '_': // OSC/DCS/SOS/PM/APC: run to BEL or ST (ESC \)
			i++
			for i < len(b) {
				if b[i] == 0x07 {
					i++
					break
				}
				if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
					i += 2
					break
				}
				i++
			}
		case '(', ')', '*', '+': // charset designation: ESC ( B — two bytes follow
			i += 2
		default: // simple two-byte escape: ESC =, ESC >, ESC M …
			i++
		}
	}
	return out.String()
}

// ScreenTextForTest returns the session's current visible screen as plain text.
func ScreenTextForTest(eng Engine, sid string) string {
	return StripANSIForTest(SerializedForTest(eng, sid))
}

// WaitScreenForTest blocks until pred is satisfied by the session's screen.
//
// It is the core synchronisation primitive of this package, and it contains no clock. On
// each iteration it checks the real observable (the screen), and if unsatisfied it blocks on
// the session's pump seam — a signal published only when the pump has FULLY processed
// another chunk of PTY output (model written, s.dirty set). Waking is therefore proof that
// the screen may have changed, and the loop re-reads it.
//
// The check-then-block order is what makes it race-free: the pump seam is 1-buffered, so a
// chunk landing between our check and our receive leaves the edge latched and wakes us
// immediately rather than being missed.
//
// The session's death channel is selected on purely so an exited shell fails fast and
// loudly instead of hanging; it is a real signal, not a deadline.
func WaitScreenForTest(
	t *testing.T,
	eng Engine,
	sid string,
	pred func(string) bool,
	what string,
) {
	t.Helper()
	notify := PumpNotifyForTest(eng, sid)
	require.NotNil(t, notify, "session %s must exist to wait on its pump", sid)
	done := SessionDoneForTest(eng, sid)
	for {
		if pred(ScreenTextForTest(eng, sid)) {
			return
		}
		select {
		case <-notify:
			// The pump processed another chunk; loop and re-read the screen.
		case <-done:
			// The shell exited. Re-check once (its final output may have satisfied pred on
			// the way out), then fail — no further output can ever arrive.
			if pred(ScreenTextForTest(eng, sid)) {
				return
			}
			t.Fatalf("session %s exited before %s\nscreen:\n%s",
				sid, what, ScreenTextForTest(eng, sid))
		}
	}
}

// WaitPromptForTest blocks until the shell has printed its prompt — i.e. it has finished
// starting up, has flushed everything it intends to say, and is now blocked reading stdin.
//
// This is the honest replacement for the old "wait for idle, then wait for output to stop
// growing for 250 ms" pair. The idle check goes true as soon as the shell is the foreground
// process group, which is BEFORE it writes its prompt; the settle window then tried to patch
// that hole by inferring quiescence from silence, which is a guess — and a guess that
// demonstrably lost, since a straggler prompt chunk arriving after the window is exactly what
// made TestMaintenance_CadenceFlush flake. The prompt has no such ambiguity: nothing follows
// it until we type.
func WaitPromptForTest(t *testing.T, eng Engine, sid string) {
	t.Helper()
	WaitScreenForTest(t, eng, sid, func(s string) bool {
		return strings.Contains(s, ShellPromptForTest)
	}, "the shell to print its first prompt")
}

// markerSeq hands out process-unique marker ids so concurrent sessions can never match on
// each other's markers.
var markerSeq atomic.Int64

// RunShellForTest writes cmd to the session and blocks until the shell has RUN it and
// returned to its prompt — leaving the session quiescent, with no output still in flight.
//
// The wait is anchored on a unique marker the shell prints after cmd. The marker is emitted
// from two arguments (`printf '%s%s\n' MK7 END`) so that the joined token "MK7END" appears
// ONLY in the command's output and never in the terminal's echo of the typed line — which
// contains "MK7 END", with a space. Matching the joined form therefore cannot be satisfied by
// the echo, so it cannot return before the command has actually run.
//
// Requiring the PROMPT after the marker is the other half: PTY output is ordered, so once the
// prompt that follows the marker is on screen, every byte the command produced has already
// been through the pump. That is what "quiescent" means, established by observation rather
// than by waiting out a timer.
func RunShellForTest(t *testing.T, eng Engine, sid, cmd string) {
	t.Helper()
	n := markerSeq.Add(1)
	tok := fmt.Sprintf("MK%dEND", n)
	line := fmt.Sprintf("%s; printf '%%s%%s\\n' MK%d END\n", cmd, n)
	require.NoError(t, eng.Write(context.Background(), sid, []byte(line)),
		"write %q to session %s", cmd, sid)
	WaitScreenForTest(t, eng, sid, func(s string) bool {
		i := strings.LastIndex(s, tok)
		return i >= 0 && strings.Contains(s[i+len(tok):], ShellPromptForTest)
	}, fmt.Sprintf("command %q to finish and the prompt to return", cmd))
}

// StartForegroundForTest starts a long-running foreground child in the session and blocks
// until the KERNEL agrees that child owns the terminal — i.e. until the session is genuinely
// non-idle by the same TIOCGPGRP measure the maintenance sweep's idle gate uses.
//
// The child is a subshell that announces itself and then execs the sleep. That shape is what
// makes the wait exact rather than approximate: job control puts a job in its own process
// group and makes it the terminal's foreground group BEFORE the job runs, and exec preserves
// that pid/pgroup for the sleep. So the marker's arrival PROVES the foreground process group
// has already flipped away from the shell — no settling, no polling, no "give it 100 ms to
// take effect". The assertion below documents that this holds with zero delay; it is
// checked, not assumed.
func StartForegroundForTest(t *testing.T, eng Engine, sid string) {
	t.Helper()
	n := markerSeq.Add(1)
	tok := fmt.Sprintf("FG%dON", n)
	line := fmt.Sprintf("( printf '%%s%%s\\n' FG%d ON; exec sleep 9999 )\n", n)
	require.NoError(t, eng.Write(context.Background(), sid, []byte(line)),
		"start foreground child in session %s", sid)
	WaitScreenForTest(t, eng, sid, func(s string) bool {
		return strings.Contains(s, tok)
	}, "the foreground child to announce itself")
	require.False(t, IsIdleForTest(eng, sid),
		"the child's first output byte must already prove it owns the terminal "+
			"(job control foregrounds a job's process group before it runs)")
}

// NewReadyShellForTest creates a session with a pinned, prompt-announcing shell and blocks
// until it is at its prompt. It is the standard opening move for any test that needs a live,
// quiescent shell to act on.
//
// Callers must have invoked PinShellForTest(t) first (it is a t.Setenv, so it must run on the
// test goroutine); the prompt this waits for would never appear if the pin had not taken.
func NewReadyShellForTest(t *testing.T, eng Engine, ws, dir string) string {
	t.Helper()
	sid, err := eng.Create(context.Background(), ws, dir, nil)
	require.NoError(t, err)
	WaitPromptForTest(t, eng, sid)
	return sid
}
