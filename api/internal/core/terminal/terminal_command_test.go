package terminal

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateCommand_RegistersSession(t *testing.T) {
	e := New()
	defer e.Shutdown()
	id, err := e.CreateCommand(context.Background(), "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 1"}, os.Environ(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.True(t, e.SessionExists(context.Background(), id))
	require.Contains(t, e.ListSessionsForWorkspace("ws1"), id)
}

// TestCreateCommand_OnExitFiresOnceSessionEnds guards the resource-leak fix
// (the agent runner concern's per-spawn tmp dir): the onExit callback passed
// to CreateCommand must fire exactly once reapOnDone has fully reaped a real,
// short-lived command's session — not before, and not more than once.
func TestCreateCommand_OnExitFiresOnceSessionEnds(t *testing.T) {
	e := New()
	defer e.Shutdown()
	ctx := context.Background()

	var fired int32
	exits := make(chan struct{}, 8) // buffered: a (buggy) duplicate fire must not block the reaper
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "true"}, os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
			exits <- struct{}{}
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	// onExit IS the signal. Block on it rather than polling for its effects: reapOnDone
	// removes the session from the registry BEFORE it invokes onExit, so once this returns
	// the deregistration is already a fact and can be asserted directly.
	<-exits
	require.False(t, e.SessionExists(ctx, id), "session must be reaped after the command exits")

	// "Exactly once" needs a moment at which no further fire is POSSIBLE — not a 50 ms window
	// in which one merely didn't happen to land. Shutdown joins every reaper before it
	// returns, so afterwards the count is final and closed. (The deferred Shutdown is a no-op
	// second call; stopOnce makes it idempotent.)
	e.Shutdown()
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must fire exactly once, never more")
}

// TestTerminateGraceful_OnExitFiresAfterGracefulSignal proves TerminateGraceful
// drives the exact same reap/cleanup chain as Kill (spec §8 must not regress
// the leak fix): the onExit callback passed to CreateCommand must still fire
// exactly once after a graceful SIGTERM lets the child exit on its own, well
// under the grace window.
func TestTerminateGraceful_OnExitFiresAfterGracefulSignal(t *testing.T) {
	e := New()
	defer e.Shutdown()
	ctx := context.Background()

	var fired int32
	exits := make(chan struct{}, 8)
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 30"}, os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
			exits <- struct{}{}
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	require.NoError(t, e.TerminateGraceful(ctx, id))

	// Block on onExit; the deregistration precedes it inside reapOnDone, so assert it directly.
	<-exits
	require.False(t, e.SessionExists(ctx, id), "session must be reaped after a graceful terminate")

	// No wall-clock assertion here. This used to do `assert.Less(elapsed, grace)` to argue
	// the `sleep` died on SIGTERM rather than the fallback SIGKILL — an upper bound that can
	// only FAIL on correct code (a loaded box that stalls the SIGTERM delivery + reap past
	// the grace fires it though nothing is wrong) and can only ever CATCH a hang that
	// `go test -timeout` catches better. That SIGTERM-vs-SIGKILL distinction is already
	// proven WITHOUT a clock by two neighbours: session_terminate_test's
	// GracefulExit_UsesSIGTERM checks the recorded exit signal IS SIGTERM, and this file's
	// FallsBackToKill companion uses a *lower* bound (elapsed >= grace) that cannot flake
	// because a timer never fires early. This test's own subject is the onExit contract:
	// fires exactly once, session reaped.

	// Shutdown joins every reaper, so afterwards no further fire is possible.
	e.Shutdown()
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must fire exactly once, never more")
}

// TestTerminateGraceful_FallsBackToKill_OnExitStillFires exercises the
// fallback-to-hard-kill branch through the full engine (not just the session
// package unit test): a child that ignores SIGTERM must still be reaped and
// its onExit fired exactly once, after consuming the (shortened, for test
// speed) grace window.
func TestTerminateGraceful_FallsBackToKill_OnExitStillFires(t *testing.T) {
	restore := SetGracefulTerminateGraceForTest(200 * time.Millisecond)
	defer restore()

	e := New()
	defer e.Shutdown()
	ctx := context.Background()

	var fired int32
	exits := make(chan struct{}, 8)
	// The child ANNOUNCES that its trap is installed, and does so from inside the shell,
	// AFTER the trap builtin has run. Waiting for that announcement is what makes this test
	// honest: the old `time.Sleep(100 * time.Millisecond)` was a guess about how fast a
	// fork+exec of /bin/sh is, and if the guess lost, the SIGTERM would race ahead of the
	// trap, the child would die normally, and the test would silently exercise the GRACEFUL
	// path while claiming to prove the FALLBACK one.
	id, err := e.CreateCommand(ctx, "ws1", t.TempDir(),
		[]string{"/bin/sh", "-c", "trap '' TERM; printf '%s%s\\n' TRAP PED; sleep 30"},
		os.Environ(), func() {
			atomic.AddInt32(&fired, 1)
			exits <- struct{}{}
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	// Block until the announcement is on the session's screen. A command session is pumped
	// and modelled exactly like a shell session, so the same screen seam applies. The token is
	// printed from two printf arguments so it cannot be matched against the argv itself.
	WaitScreenForTest(t, e, id, func(s string) bool {
		return strings.Contains(s, "TRAPPED")
	}, "the child to install its SIGTERM trap")

	start := time.Now()
	require.NoError(t, e.TerminateGraceful(ctx, id))
	elapsed := time.Since(start)

	<-exits
	require.False(t, e.SessionExists(ctx, id),
		"session must still be reaped when the child ignores SIGTERM")

	// TIMING-BY-SUBJECT: the grace window IS the behaviour under test. This asserts the
	// signal-ignoring child consumed the full (deliberately shortened, via the
	// SetGracefulTerminateGraceForTest seam) window before the fallback hard-kill — an
	// observation about elapsed time, not a wait on one.
	assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond,
		"a signal-ignoring child must consume the full (shortened) grace window")

	e.Shutdown()
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must fire exactly once, never more")
}

// TestEngine_TerminateGraceful_Unknown mirrors TestEngine_Kill_Unknown: an
// unknown session id must return a wrapped ErrSessionNotFound, not panic or
// silently succeed.
func TestEngine_TerminateGraceful_Unknown(t *testing.T) {
	e := New()
	defer e.Shutdown()
	err := e.TerminateGraceful(context.Background(), "nope")
	require.Error(t, err)
}

func TestWithTerminalDefaults_InjectsTERMWhenAbsent(t *testing.T) {
	// A command session started with an env lacking TERM ends up with the default.
	got := withTerminalDefaults([]string{"PATH=/usr/bin"})
	require.Contains(t, got, "TERM=xterm-256color")
	require.Contains(t, got, "COLORTERM=truecolor")
	// Does not override a caller-provided TERM.
	got = withTerminalDefaults([]string{"TERM=screen"})
	require.Contains(t, got, "TERM=screen")
	require.NotContains(t, got, "TERM=xterm-256color")
}

// TestWithTerminalDefaults_InjectsUTF8LocaleWhenAbsent: the backfill used to stop at
// TERM, so a CreateCommand child (every agent-chat vendor CLI) inherited launchd's
// locale-less environment while the interactive terminal got ptyEnv()'s UTF-8 LANG.
// A CLI that copies through pbcopy then had its UTF-8 read as Mac OS Roman —
// __CF_USER_TEXT_ENCODING's script code 0 — putting "‚Äî" on the pasteboard for "—"
// while the rendered screen stayed correct.
func TestWithTerminalDefaults_InjectsUTF8LocaleWhenAbsent(t *testing.T) {
	got := withTerminalDefaults([]string{"PATH=/usr/bin"})
	require.Contains(t, got, "LANG="+defaultLocale(nil, runtime.GOOS))
	// The injected value must actually be a UTF-8 locale, whatever the platform.
	var lang string
	for _, kv := range got {
		if strings.HasPrefix(kv, "LANG=") {
			lang = kv
		}
	}
	require.Contains(t, lang, "UTF-8")
}

// TestRegression_CreateCommandChildGetsUTF8Locale is the end-to-end half of the
// clipboard-mojibake fix, and the reason it is a spawn rather than another table
// case: what broke in the field was not the decision but its DELIVERY — the child
// process actually reading LANG out of its own environment. It spawns under a
// launchd-minimal env (PATH and nothing else, exactly what a GUI-launched daemon
// hands down) and has the shell report the LANG it really received.
func TestRegression_CreateCommandChildGetsUTF8Locale(t *testing.T) {
	e := New()
	defer e.Shutdown()

	dir := t.TempDir()
	out := filepath.Join(dir, "lang")
	exits := make(chan struct{}, 1)
	_, err := e.CreateCommand(context.Background(), "ws-locale", dir,
		[]string{"/bin/sh", "-c", "printf '%s' \"$LANG\" > \"" + out + "\""},
		[]string{"PATH=/usr/bin:/bin"},
		func() { exits <- struct{}{} })
	require.NoError(t, err)

	// onExit IS the signal: it fires only after reapOnDone has seen the process die,
	// by which point the redirect above has already been flushed and closed.
	<-exits

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Contains(t, string(got), "UTF-8",
		"a CreateCommand child inheriting no locale must still get a UTF-8 LANG; "+
			"without it pbcopy reads the CLI's UTF-8 as Mac OS Roman and the clipboard mojibakes")
}

// TestWithTerminalDefaults_NeverOverridesCallerLocale: defaultLocale backs off if ANY
// of LANG/LC_ALL/LC_CTYPE is set, so a user who deliberately runs a non-UTF-8 (or
// simply a different) locale keeps it — the backfill only fills a total vacuum.
func TestWithTerminalDefaults_NeverOverridesCallerLocale(t *testing.T) {
	for _, set := range []string{
		"LANG=en_GB.ISO-8859-1",
		"LC_ALL=fr_FR.UTF-8",
		"LC_CTYPE=de_DE.UTF-8",
	} {
		got := withTerminalDefaults([]string{"PATH=/usr/bin", set})
		require.Contains(t, got, set)
		require.NotContains(t, got, "LANG=en_US.UTF-8")
		require.NotContains(t, got, "LANG=C.UTF-8")
	}
}

// TestEngine_CreateCommand_MissingBinary_IsErrCommandNotFound: a vendor CLI that is
// not installed must come back as the CLASSIFIED sentinel, not as raw exec text a
// caller would have to string-match. This is the failure the packaged .app hit on
// every single chat (claude lives in ~/.local/bin, which launchd's PATH omits), and
// it reached the user as an unmapped 500 and a button that did nothing.
func TestEngine_CreateCommand_MissingBinary_IsErrCommandNotFound(t *testing.T) {
	e := New()
	defer e.Shutdown()

	_, err := e.CreateCommand(
		context.Background(),
		"ws1",
		t.TempDir(),
		[]string{"crowbar-definitely-not-a-real-binary-xyz"},
		[]string{"PATH=/usr/bin"},
		nil,
	)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrCommandNotFound)
}
