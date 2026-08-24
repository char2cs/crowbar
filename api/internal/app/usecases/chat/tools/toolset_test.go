package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/tools"
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

func toolsetOn(t *testing.T, renamer tools.ChatRenamer) (*tools.ToolSet, string) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	tok := m.Mint("RUN")
	// EVERY port is wired here (with empty-returning stubs) so the shared toolset
	// fixture always advertises every registered tool group — which is what makes
	// TestToolSet_RespectsToolCeiling and TestToolSet_NoToolAcceptsAScopeArgument
	// below guard the whole surface rather than just set_chat_title. A port left
	// out here silently narrows both guards to the tools that happen to remain,
	// which is how they were vacuous before.
	deps := tools.Deps{
		Resolver:        res,
		Chats:           renamer,
		Threads:         &stubThreadReader{},
		Review:          &stubReviewReader{},
		ThreadWrites:    &stubThreadWriter{},
		Idempotency:     tools.NewIdempotency(),
		ChatReads:       stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		ChatLogs:        &stubChatLogs{},
		Lineage:         &stubLineage{},
		ThreadBroadcast: (&spyThreadBroadcast{}).fn(),
	}
	return tools.NewToolSet(deps, "RUN", tok), tok
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
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		stubChats{}, stubWorkspaces{all: tree()})
	ts := tools.NewToolSet(tools.Deps{Resolver: res, Chats: spy}, "RUN", "forged")

	_, err = ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))
	require.ErrorIs(t, err, tools.ErrUnauthorized)
	require.Zero(t, spy.calls, "an unauthorized call must never reach a tool handler")
}

// ── the per-provider tool switch ────────────────────────────────────
// Deps.ToolAccess is the user's "Tools" switch, and it is consulted on EVERY
// call rather than once at spawn. The tests here pin the three answers the port
// can give; the live end-to-end behaviour (a switch flipped underneath a running
// chat) is pinned against the real agent provider concern in
// TestDispatchMCP_ToolCallIsRefusedOnceToolsAreSwitchedOff.

// toolsetGatedOn builds the full surface with a ToolAccess port under the test's
// control, recording every provider it was asked about — which is what proves
// the question is asked about the RESOLVED caller's provider rather than about
// something the relay could have supplied.
func toolsetGatedOn(
	t *testing.T,
	renamer tools.ChatRenamer,
	access func(providerID string) (bool, error),
) (*tools.ToolSet, *[]string) {
	t.Helper()
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{
			ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a", ProviderID: "codex",
		}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}},
		stubWorkspaces{all: tree()})
	asked := &[]string{}
	deps := tools.Deps{
		Resolver: res,
		Chats:    renamer,
		ToolAccess: func(_ context.Context, providerID string) (bool, error) {
			*asked = append(*asked, providerID)
			return access(providerID)
		},
	}
	return tools.NewToolSet(deps, "RUN", m.Mint("RUN")), asked
}

func TestToolSet_RefusesEveryCallWhenTheProvidersToolsAreOff(t *testing.T) {
	spy := &spyRenamer{}
	ts, asked := toolsetGatedOn(t, spy, func(string) (bool, error) { return false, nil })

	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))

	require.ErrorIs(t, err, tools.ErrToolsDisabled)
	require.Zero(t, spy.calls, "a refused call must never reach a tool handler")
	require.Equal(t, []string{"codex"}, *asked,
		"the switch must be read for the RESOLVED caller's provider")
}

func TestToolSet_ServesTheCallWhenTheProvidersToolsAreOn(t *testing.T) {
	spy := &spyRenamer{}
	ts, asked := toolsetGatedOn(t, spy, func(string) (bool, error) { return true, nil })

	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"Fine"}`))

	require.NoError(t, err)
	require.Equal(t, 1, spy.calls)
	require.Equal(t, []string{"codex"}, *asked)
}

// A preference the daemon cannot READ must not be guessed at in the permissive
// direction: the whole point of the switch is that the user decided, and a
// storage failure is not permission.
func TestToolSet_RefusesWhenTheSwitchCannotBeRead(t *testing.T) {
	spy := &spyRenamer{}
	ts, _ := toolsetGatedOn(t, spy, func(string) (bool, error) {
		return false, errors.New("provider preference store is down")
	})

	_, err := ts.Call(context.Background(), "set_chat_title", json.RawMessage(`{"title":"x"}`))

	require.Error(t, err)
	require.Zero(t, spy.calls, "an unreadable preference must fail closed")
}

// The switch is consulted BEFORE the caller is even known to be calling a real
// tool, so a provider with its tools off gets the same answer whatever it asks
// for — including a tool that does not exist. The alternative leaks which tools
// this daemon registers to a caller that may not use any of them.
func TestToolSet_ADisabledProviderCannotProbeForToolNames(t *testing.T) {
	ts, _ := toolsetGatedOn(t, &spyRenamer{}, func(string) (bool, error) { return false, nil })

	_, err := ts.Call(context.Background(), "rm_rf", json.RawMessage(`{}`))

	require.ErrorIs(t, err, tools.ErrToolsDisabled)
}
