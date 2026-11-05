//go:build unix

package session

import (
	"errors"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal/internal/model"
)

// swapPtyForegroundPgid replaces the foreground-pgid probe seam for the test's duration.
func swapPtyForegroundPgid(t *testing.T, fn func(uintptr) (int, error)) {
	t.Helper()
	orig := ptyForegroundPgid
	ptyForegroundPgid = fn
	t.Cleanup(func() { ptyForegroundPgid = orig })
}

// swapGetpgidFn replaces the getpgid seam for the test's duration.
func swapGetpgidFn(t *testing.T, fn func(int) (int, error)) {
	t.Helper()
	orig := getpgidFn
	getpgidFn = fn
	t.Cleanup(func() { getpgidFn = orig })
}

// TestPtyForegroundPgid_IoctlError covers the ptyForegroundPgid errno branch: a closed fd
// makes the TIOCGPGRP ioctl fail, returning an error.
func TestPtyForegroundPgid_IoctlError(t *testing.T) {
	f := closedFile(t)
	_, err := ptyForegroundPgid(f.Fd())
	require.Error(t, err, "TIOCGPGRP on a closed fd must return an error")
}

// fgSession builds a non-live-PTY-pump bare session that nonetheless passes the
// ptmx/cmd/Process non-nil guards in isIdleLocked/checkForegroundResetLocked, so the syscall
// seams decide the outcome.
func fgSession(t *testing.T) *Session {
	t.Helper()
	s := newBareSession("sid-fg", "/bin/sh", t.TempDir(), "")
	s.ptmx = closedFile(t)
	s.cmd = &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}
	s.model = &fakeModel{cols: 80, rows: 24}
	return s
}

// TestIsIdleLocked_IoctlError covers isIdleLocked's ioctl-error branch via the seam.
func TestIsIdleLocked_IoctlError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 0, errors.New("ioctl boom") })
	s := fgSession(t)
	assert.False(t, s.IsIdle(), "an ioctl error must make isIdleLocked report not-idle")
}

// TestIsIdleLocked_GetpgidError covers isIdleLocked's getpgid-error branch via the seam.
func TestIsIdleLocked_GetpgidError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 4242, nil })
	swapGetpgidFn(t, func(int) (int, error) { return 0, errors.New("getpgid boom") })
	s := fgSession(t)
	assert.False(t, s.IsIdle(), "a getpgid error must make isIdleLocked report not-idle")
}

// TestCheckForegroundResetLocked_IoctlError covers checkForegroundResetLocked's ioctl-error
// early return: the model's OnForegroundReset must NOT fire when the probe fails.
func TestCheckForegroundResetLocked_IoctlError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 0, errors.New("ioctl boom") })
	s := fgSession(t)
	rec := &resetRecorder{}
	s.model = rec
	s.mu.Lock()
	s.checkForegroundResetLocked()
	s.mu.Unlock()
	assert.False(t, rec.reset, "an ioctl error must skip OnForegroundReset")
}

// TestCheckForegroundResetLocked_GetpgidError covers checkForegroundResetLocked's
// getpgid-error early return.
func TestCheckForegroundResetLocked_GetpgidError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 4242, nil })
	swapGetpgidFn(t, func(int) (int, error) { return 0, errors.New("getpgid boom") })
	s := fgSession(t)
	rec := &resetRecorder{}
	s.model = rec
	s.mu.Lock()
	s.checkForegroundResetLocked()
	s.mu.Unlock()
	assert.False(t, rec.reset, "a getpgid error must skip OnForegroundReset")
}

// resetRecorder is an inert model that records whether OnForegroundReset fired.
type resetRecorder struct {
	fakeModel
	reset bool
}

func (m *resetRecorder) OnForegroundReset() { m.reset = true }

var _ model.TerminalModel = (*resetRecorder)(nil)
