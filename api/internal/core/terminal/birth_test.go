package terminal_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestEngine_Create_BadShell(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	_, err := eng.Create(ctx, "chat-bad", dir, nil)
	// With no profile, it uses $SHELL or /bin/sh which is valid, so this test
	// exercises the profile resolution path. To trigger an error we need a bad profile.
	require.NoError(t, err) // default shell succeeds
	_ = eng.Kill(ctx, eng.ListSessions()[0])
}

func TestEngine_Create_InvalidShellViaProfile(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	prof := &domain.TerminalProfile{Shell: "/nonexistent/shell"}
	_, err := eng.Create(ctx, "chat-bad", dir, prof)
	assert.Error(t, err)
}

func TestEngine_Create_WithStartupCommands(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	ctx := context.Background()
	dir := t.TempDir()

	prof := &domain.TerminalProfile{
		ID:              "prof-startup",
		StartupCommands: []string{"echo startup-test"},
	}

	sid, err := eng.Create(ctx, "chat-1", dir, prof)
	require.NoError(t, err)
	require.NoError(t, eng.Kill(ctx, sid))
}

func TestCreateCommand_OnExitCarriesCodeAndFinalScreen(t *testing.T) {
	eng := terminal.New()
	terminal.StopMaintenanceForTest(eng)
	defer eng.Shutdown()
	exits := make(chan terminal.Exit, 1)

	_, err := eng.CreateCommand(context.Background(), "chat-exit", t.TempDir(),
		[]string{"/bin/sh", "-c", "echo bye; exit 3"}, nil, func(e terminal.Exit) { exits <- e })
	require.NoError(t, err)

	select {
	case exit := <-exits:
		assert.Equal(t, 3, exit.Code)
		assert.Contains(t, exit.Screen, "bye", "the screen the process left behind")
	case <-time.After(10 * time.Second):
		t.Fatal("onExit never ran for a process that exited")
	}
}
