package session

import (
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewCommand_RunsArgv(t *testing.T) {
	dir := t.TempDir()
	s, err := NewCommand(t.Context(), "cmd-id", []string{"/bin/sh", "-c", "printf CMDMARKER; sleep 9999"}, dir, os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)
	require.Equal(t, "cmd-id", s.ID())

	ch, err := s.Attach()
	require.NoError(t, err)

	waitFrameContaining(t, ch, "CMDMARKER")
}

func TestNewCommand_KeepsTheBracketedPasteItsAppSetsAtStartup(t *testing.T) {
	s, err := NewCommand(t.Context(), "cmd-paste", []string{"/bin/sh", "-c", `printf '\033[?2004hPASTEREADY'; sleep 9999`},
		t.TempDir(), os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	notify := s.PumpNotifyForTest()
	for {
		st, ok := s.ScreenState()
		require.True(t, ok)
		if strings.Contains(st.Text, "PASTEREADY") {
			assert.True(t, st.BracketedPaste)
			return
		}
		select {
		case <-notify:
		case <-s.Done():
			t.Fatal("the command exited before painting")
		}
	}
}

func TestNewCommand_EmptyArgvErrors(t *testing.T) {
	_, err := NewCommand(t.Context(), "x", nil, t.TempDir(), os.Environ(), 80, 24, 0)
	require.Error(t, err)
}

func TestNewCommand_IsCommandAndNeverSuspendEligible(t *testing.T) {
	dir := t.TempDir()
	s, err := NewCommand(t.Context(), "cmd-suspend-guard", []string{"/bin/sh", "-c", "sleep 30"}, dir, os.Environ(), 80, 24, 0)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	assert.True(t, s.IsCommand(), "a NewCommand session must report IsCommand()==true")
	assert.False(t, s.SuspendEligible(false), "a command session must never be idle-suspend eligible")
	assert.False(t, s.SuspendEligible(true), "a command session must never be force-suspend eligible")
}

func TestNew_ShellSession_IsNotCommand_StillSuspendEligible(t *testing.T) {
	dir := t.TempDir()
	s, err := newTestSession(t, "shell-suspend-guard", dir)
	require.NoError(t, err)
	t.Cleanup(s.Kill)

	assert.False(t, s.IsCommand(), "a New() shell session must report IsCommand()==false")

	waitIdlePrompt(t, s)
	assert.True(t, s.SuspendEligible(false), "an idle, detached shell session must remain suspend-eligible")
}
