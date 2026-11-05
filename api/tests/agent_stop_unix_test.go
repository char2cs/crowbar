//go:build integration && unix

package tests

import (
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/tests/kit"
)

// A replacement launches only once the CLI it replaces is gone: the wait is on
// the old process's exit, never a clock.
func TestRegression_AReplacementNeverLandsBesideTheCLIItReplaces(t *testing.T) {
	h := newHarness(t)
	imported := importWritableWorkspace(t, h)
	fake := kit.InstallFakeProvider(t, h.home, kit.FakeSpec{Base: "claude"})
	chatID, l1 := createFakeChat(t, h, imported, fake)
	s := argAfter(l1.Argv, "--session-id")
	require.NotEmpty(t, s, "the first launch names its session: %v", l1.Argv)
	completeTurn(t, h, l1, h.url+repoBase(imported), "claude", s, "one")

	l2 := promptLaunch(t, h, imported, fake, chatID, "two", "")

	require.NotEqual(t, l1.RunnerID, l2.RunnerID, "precondition: prompt two restarted the CLI")
	assert.ErrorIs(t, syscall.Kill(l1.PID, 0), syscall.ESRCH,
		"launch 2 exists, so launch 1's process must already be gone")
}
