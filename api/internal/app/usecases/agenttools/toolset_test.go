package agenttools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type spyRenamer struct {
	runnerID, title, source string
	calls                   int
}

func (s *spyRenamer) RenameByRunner(_ context.Context, runnerID, title, source string) error {
	s.calls++
	s.runnerID, s.title, s.source = runnerID, title, source
	return nil
}

func toolsetOn(t *testing.T, renamer agenttools.ChatRenamer) (*agenttools.ToolSet, string) {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	tok := m.Mint("RUN")
	// EVERY port is wired here (with empty-returning stubs) so the shared toolset
	// fixture always advertises every registered tool group — which is what makes
	// TestToolSet_RespectsToolCeiling and TestToolSet_NoToolAcceptsAScopeArgument
	// below guard the whole surface rather than just set_chat_title. A port left
	// out here silently narrows both guards to the tools that happen to remain,
	// which is how they were vacuous before.
	deps := agenttools.Deps{
		Resolver:        res,
		Chats:           renamer,
		Threads:         &stubThreadReader{},
		Review:          &stubReviewReader{},
		ThreadWrites:    &stubThreadWriter{},
		Idempotency:     agenttools.NewIdempotency(),
		ChatReads:       stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: "ws-a"}},
		ChatLogs:        &stubChatLogs{},
		ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
	}
	return agenttools.NewToolSet(deps, "RUN", tok), tok
}

func TestToolSet_AdvertisesSetChatTitle(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	names := []string{}
	for _, tool := range ts.Tools() {
		names = append(names, tool.Name)
		require.NotEmpty(t, tool.Description, "%s has no description — it is the whole trigger budget", tool.Name)
		require.NotEmpty(t, tool.InputSchema)
	}
	require.Contains(t, names, "set_chat_title")
}

// Global constraint: codex does not defer tool schemas, so every tool costs
// context on every codex turn.
//
// Exactly 8, not "at most 8". A LessOrEqual here is a guard that cannot fail
// for the reason it exists: it passes just as happily on a fixture that wires
// five tools as on one that wires all eight — which is precisely how a
// 5-of-8 toolsetOn hid for three tasks while this test stayed green, silently
// narrowing every other guard built on the same fixture to the tools that
// happened to remain.
func TestToolSet_RespectsToolCeiling(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	require.Len(t, ts.Tools(), 8)
}

// No tool may take a scope argument — authority comes from the runner, never
// from something the model can type.
func TestToolSet_NoToolAcceptsAScopeArgument(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	forbidden := []string{"workspaceId", "workspace_id", "projectId", "project_id", "repoId", "repo_id", "runnerId", "segment"}
	for _, tool := range ts.Tools() {
		for _, f := range forbidden {
			require.NotContains(t, string(tool.InputSchema), f,
				"tool %s exposes %s; scope must never be an argument", tool.Name, f)
		}
	}
}

func TestToolSet_SetChatTitleRenamesTheCallersRunner(t *testing.T) {
	spy := &spyRenamer{}
	ts, _ := toolsetOn(t, spy)

	out, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"Refactor auth"}`))
	require.NoError(t, err)
	require.Contains(t, out, "Refactor auth")

	require.Equal(t, 1, spy.calls)
	require.Equal(t, "RUN", spy.runnerID)
	require.Equal(t, "Refactor auth", spy.title)
	// source=agent so a user-locked title is never clobbered.
	require.Equal(t, "agent", spy.source)
}

func TestToolSet_SetChatTitleRejectsEmptyTitle(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"   "}`))
	require.Error(t, err)
}

func TestToolSet_UnknownToolErrors(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	_, err := ts.Call(context.Background(), "rm_rf", json.RawMessage(`{}`))
	require.Error(t, err)
}

func TestToolSet_BadTokenCannotReachAnyTool(t *testing.T) {
	spy := &spyRenamer{}
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{}, stubWorkspaces{all: tree()})
	ts := agenttools.NewToolSet(agenttools.Deps{Resolver: res, Chats: spy}, "RUN", "forged")

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.ErrorIs(t, err, agenttools.ErrUnauthorized)
	require.Zero(t, spy.calls, "an unauthorized call must never reach a tool handler")
}
