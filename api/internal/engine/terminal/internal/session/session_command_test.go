package session

import (
	"os"
	"strings"
	"testing"
	"time"

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

func containsBytes(b []byte, sub string) bool { return len(b) > 0 && (string(b) == sub || indexOfBytes(b, sub) >= 0) }

func indexOfBytes(b []byte, sub string) int {
	return strings.Index(string(b), sub)
}
