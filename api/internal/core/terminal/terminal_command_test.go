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
	id, err := e.CreateCommand(context.Background(), "chat1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 1"}, os.Environ(), nil)
	require.NoError(t, err)
	require.NotEmpty(t, id)
	require.True(t, e.SessionExists(context.Background(), id))
	require.Contains(t, e.ListSessionsForChat("chat1"), id)
}

func TestCreateCommand_OnExitFiresOnceSessionEnds(t *testing.T) {
	e := New()
	defer e.Shutdown()
	ctx := context.Background()

	var fired int32
	exits := make(chan struct{}, 8)
	id, err := e.CreateCommand(ctx, "chat1", t.TempDir(),
		[]string{"/bin/sh", "-c", "true"}, os.Environ(), func(Exit) {
			atomic.AddInt32(&fired, 1)
			exits <- struct{}{}
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	<-exits
	require.False(t, e.SessionExists(ctx, id), "session must be reaped after the command exits")

	e.Shutdown()
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must fire exactly once, never more")
}

func TestTerminateGraceful_OnExitFiresAfterGracefulSignal(t *testing.T) {
	e := New()
	defer e.Shutdown()
	ctx := context.Background()

	var fired int32
	exits := make(chan struct{}, 8)
	id, err := e.CreateCommand(ctx, "chat1", t.TempDir(),
		[]string{"/bin/sh", "-c", "sleep 30"}, os.Environ(), func(Exit) {
			atomic.AddInt32(&fired, 1)
			exits <- struct{}{}
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	require.NoError(t, e.TerminateGraceful(ctx, id))

	<-exits
	require.False(t, e.SessionExists(ctx, id), "session must be reaped after a graceful terminate")

	e.Shutdown()
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must fire exactly once, never more")
}

func TestTerminateGraceful_FallsBackToKill_OnExitStillFires(t *testing.T) {
	e := New()
	defer e.Shutdown()
	restore := SetGracefulTerminateGraceForTest(e, 200*time.Millisecond)
	defer restore()
	ctx := context.Background()

	var fired int32
	exits := make(chan struct{}, 8)
	id, err := e.CreateCommand(ctx, "chat1", t.TempDir(),
		[]string{"/bin/sh", "-c", "trap '' TERM; printf '%s%s\\n' TRAP PED; sleep 30"},
		os.Environ(), func(Exit) {
			atomic.AddInt32(&fired, 1)
			exits <- struct{}{}
		})
	require.NoError(t, err)
	require.NotEmpty(t, id)

	WaitScreenForTest(t, e, id, func(s string) bool {
		return strings.Contains(s, "TRAPPED")
	}, "the child to install its SIGTERM trap")

	start := time.Now()
	require.NoError(t, e.TerminateGraceful(ctx, id))
	elapsed := time.Since(start)

	<-exits
	require.False(t, e.SessionExists(ctx, id),
		"session must still be reaped when the child ignores SIGTERM")

	assert.GreaterOrEqual(t, elapsed, 200*time.Millisecond,
		"a signal-ignoring child must consume the full (shortened) grace window")

	e.Shutdown()
	require.EqualValues(t, 1, atomic.LoadInt32(&fired), "onExit must fire exactly once, never more")
}

func TestEngine_TerminateGraceful_Unknown(t *testing.T) {
	e := New()
	defer e.Shutdown()
	err := e.TerminateGraceful(context.Background(), "nope")
	require.Error(t, err)
}

func TestWithTerminalDefaults_InjectsTERMWhenAbsent(t *testing.T) {
	got := withTerminalDefaults([]string{"PATH=/usr/bin"})
	require.Contains(t, got, "TERM=xterm-256color")
	require.Contains(t, got, "COLORTERM=truecolor")
	got = withTerminalDefaults([]string{"TERM=screen"})
	require.Contains(t, got, "TERM=screen")
	require.NotContains(t, got, "TERM=xterm-256color")
}

func TestWithTerminalDefaults_InjectsUTF8LocaleWhenAbsent(t *testing.T) {
	got := withTerminalDefaults([]string{"PATH=/usr/bin"})
	require.Contains(t, got, "LANG="+defaultLocale(nil, runtime.GOOS))
	var lang string
	for _, kv := range got {
		if strings.HasPrefix(kv, "LANG=") {
			lang = kv
		}
	}
	require.Contains(t, lang, "UTF-8")
}

func TestRegression_CreateCommandChildGetsUTF8Locale(t *testing.T) {
	e := New()
	defer e.Shutdown()

	dir := t.TempDir()
	out := filepath.Join(dir, "lang")
	exits := make(chan struct{}, 1)
	_, err := e.CreateCommand(context.Background(), "chat-locale", dir,
		[]string{"/bin/sh", "-c", "printf '%s' \"$LANG\" > \"" + out + "\""},
		[]string{"PATH=/usr/bin:/bin"},
		func(Exit) { exits <- struct{}{} })
	require.NoError(t, err)

	<-exits

	got, err := os.ReadFile(out)
	require.NoError(t, err)
	require.Contains(t, string(got), "UTF-8",
		"a CreateCommand child inheriting no locale must still get a UTF-8 LANG; "+
			"without it pbcopy reads the CLI's UTF-8 as Mac OS Roman and the clipboard mojibakes")
}

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

func TestEngine_CreateCommand_MissingBinary_IsErrCommandNotFound(t *testing.T) {
	e := New()
	defer e.Shutdown()

	_, err := e.CreateCommand(
		context.Background(),
		"chat1",
		t.TempDir(),
		[]string{"crowbar-definitely-not-a-real-binary-xyz"},
		[]string{"PATH=/usr/bin"},
		nil,
	)

	require.Error(t, err)
	require.ErrorIs(t, err, ErrCommandNotFound)
}
