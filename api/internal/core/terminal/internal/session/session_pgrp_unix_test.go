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

func swapPtyForegroundPgid(t *testing.T, fn func(uintptr) (int, error)) {
	t.Helper()
	orig := ptyForegroundPgid
	ptyForegroundPgid = fn
	t.Cleanup(func() { ptyForegroundPgid = orig })
}

func swapGetpgidFn(t *testing.T, fn func(int) (int, error)) {
	t.Helper()
	orig := getpgidFn
	getpgidFn = fn
	t.Cleanup(func() { getpgidFn = orig })
}

func TestPtyForegroundPgid_IoctlError(t *testing.T) {
	f := closedFile(t)
	_, err := ptyForegroundPgid(f.Fd())
	require.Error(t, err, "TIOCGPGRP on a closed fd must return an error")
}

func fgSession(t *testing.T) *Session {
	t.Helper()
	s := newBareSession("sid-fg", "/bin/sh", t.TempDir(), "")
	s.ptmx = closedFile(t)
	s.cmd = &exec.Cmd{Process: &os.Process{Pid: os.Getpid()}}
	s.model = &fakeModel{cols: 80, rows: 24}
	return s
}

func TestIsIdleLocked_IoctlError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 0, errors.New("ioctl boom") })
	s := fgSession(t)
	assert.False(t, s.IsIdle(), "an ioctl error must make isIdleLocked report not-idle")
}

func TestIsIdleLocked_GetpgidError(t *testing.T) {
	swapPtyForegroundPgid(t, func(uintptr) (int, error) { return 4242, nil })
	swapGetpgidFn(t, func(int) (int, error) { return 0, errors.New("getpgid boom") })
	s := fgSession(t)
	assert.False(t, s.IsIdle(), "a getpgid error must make isIdleLocked report not-idle")
}

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

type resetRecorder struct {
	fakeModel
	reset bool
}

func (m *resetRecorder) OnForegroundReset() { m.reset = true }

var _ model.TerminalModel = (*resetRecorder)(nil)
