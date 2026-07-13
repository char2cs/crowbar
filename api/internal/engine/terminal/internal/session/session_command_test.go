package session

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand_RunsArgv(t *testing.T) {
	dir := t.TempDir()
	// `sh -c 'printf MARKER; sleep …'` — a real argv with args, not a bare shell.
	//
	// The trailing sleep is a LIVENESS vehicle, not a wait: Attach errors on a session whose
	// child has already exited, so the child must outlive the Attach below. The old `sleep 0.2`
	// made that a race — a 200ms bet that this test goroutine gets scheduled in time. Sleeping
	// effectively forever instead removes the bet entirely; the child is reaped by Kill, and
	// the marker is observed through the frames, not through the sleep's length.
	s, err := NewCommand("cmd-id", []string{"/bin/sh", "-c", "printf CMDMARKER; sleep 9999"}, dir, os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	require.Equal(t, "cmd-id", s.ID())

	ch, err := s.Attach()
	require.NoError(t, err)

	// Block on the frames until the marker shows. It reaches this client either live (the pump
	// fanned it out after the attach) or inside the attach snapshot serialized from the model —
	// both are real deliveries of the command's output, and neither needs a deadline.
	waitFrameContaining(t, ch, "CMDMARKER")
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

	waitIdlePrompt(t, s)
	assert.True(t, s.BeginSuspendIfEligible(), "an idle, detached shell session must remain suspend-eligible")
}
