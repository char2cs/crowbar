package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand_RunsArgv(t *testing.T) {
	dir := t.TempDir()
	// `sh -c 'printf MARKER'` — a real argv with args, not a bare shell.
	s, err := NewCommand("cmd-id", []string{"/bin/sh", "-c", "printf CMDMARKER; sleep 0.2"}, dir, os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	require.Equal(t, "cmd-id", s.ID())

	ch, err := s.Attach()
	require.NoError(t, err)
	deadline := time.After(3 * time.Second)
	var seen bool
	for !seen {
		select {
		case f := <-ch:
			if containsBytes(f.Data, "CMDMARKER") {
				seen = true
			}
		case <-deadline:
			t.Fatal("did not observe command output")
		}
	}
	require.True(t, seen)
	s.Kill()
}

func TestNewCommand_EmptyArgvErrors(t *testing.T) {
	_, err := NewCommand("x", nil, t.TempDir(), os.Environ(), 80, 24, 0)
	require.Error(t, err)
}

// TestNewCommand_IsCommandAndNeverSuspendEligible guards the fatal agentic-CLI bug: a
// command session must report IsCommand()==true and must be rejected by BOTH suspend
// entry points regardless of its idle/attached state — Suspend's PTY teardown would
// kill the vendor process outright, and restore cannot bring it back (it would
// exec.Command the joined argv string, not the original binary).
func TestNewCommand_IsCommandAndNeverSuspendEligible(t *testing.T) {
	dir := t.TempDir()
	// A long-running, definitely-detached, definitely-idle-looking process: even so,
	// the command guard must short-circuit before any idle/client check.
	s, err := NewCommand("cmd-suspend-guard", []string{"/bin/sh", "-c", "sleep 30"}, dir, os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	assert.True(t, s.IsCommand(), "a NewCommand session must report IsCommand()==true")
	assert.False(t, s.BeginSuspendIfEligible(), "a command session must never be idle-suspend eligible")
	assert.False(t, s.BeginForceSuspend(), "a command session must never be force-suspend eligible")
	assert.False(t, s.Suspending(), "the suspending flag must remain unset after rejected suspend attempts")
}

// TestNew_ShellSession_IsNotCommand_StillSuspendEligible is the regression guard for the
// command-session carve-out: a plain login-shell session (New, not NewCommand) must
// report IsCommand()==false and must remain suspend-eligible once idle+detached, exactly
// as before this fix.
func TestNew_ShellSession_IsNotCommand_StillSuspendEligible(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "shell-suspend-guard", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	assert.False(t, s.IsCommand(), "a New() shell session must report IsCommand()==false")

	if !waitIdleOrSkip(t, s) {
		return
	}
	assert.True(t, s.BeginSuspendIfEligible(), "an idle, detached shell session must remain suspend-eligible")
}

func containsBytes(b []byte, sub string) bool { return len(b) > 0 && (string(b) == sub || indexOfBytes(b, sub) >= 0) }

func indexOfBytes(b []byte, sub string) int {
	return strings.Index(string(b), sub)
}
