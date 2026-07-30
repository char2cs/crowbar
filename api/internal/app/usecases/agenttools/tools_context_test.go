package agenttools_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// stubChatsByWorkspace answers ListByWorkspace per workspace id, unlike
// stubChats (which always returns the same fixed list regardless of id) — it
// is what list_workspaces' tests need to prove each workspace's chats are
// folded in separately rather than one list applied to every workspace.
type stubChatsByWorkspace struct {
	byWS map[string][]domain.AgentChat
}

func (s stubChatsByWorkspace) Get(context.Context, string) (domain.AgentChat, error) {
	return domain.AgentChat{}, nil
}

func (s stubChatsByWorkspace) ListByWorkspace(
	_ context.Context,
	wsID string,
) ([]domain.AgentChat, error) {
	return s.byWS[wsID], nil
}

func listWorkspacesToolsOn(
	t *testing.T,
	callerWs string,
	byWS map[string][]domain.AgentChat,
) *agenttools.ToolSet {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()})
	return agenttools.NewToolSet(agenttools.Deps{
		Resolver: res, ChatReads: stubChatsByWorkspace{byWS: byWS},
	}, "RUN", m.Mint("RUN"))
}

// workspaceHeaders extracts the unindented header lines renderWorkspaces emits
// — one per visible workspace, "* " prefixed for the caller's own — so a test
// can assert on exactly the workspace set without a chat row (indented, and
// carrying free-typed title text) being mistaken for one. Without this, a
// substring check alone could not tell "ws-a" from "ws-a1".
func workspaceHeaders(out string) []string {
	var headers []string
	for _, line := range strings.Split(out, "\n") {
		if line == "" || strings.HasPrefix(line, " ") {
			continue
		}
		headers = append(headers, line)
	}
	return headers
}

func TestListWorkspaces_ListsOnlyTheVisibleSetAndMarksSelf(t *testing.T) {
	ts := listWorkspacesToolsOn(t, "ws-a", nil)

	out, err := ts.Call(context.Background(), "list_workspaces", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"* ws-a", "ws-a1"}, workspaceHeaders(out))
}

func TestListWorkspaces_IncludesEachWorkspacesChats(t *testing.T) {
	byWS := map[string][]domain.AgentChat{
		"ws-a":  {{ID: "c1", Title: "Fix Auth Bug"}},
		"ws-a1": {{ID: "c2", Title: "Refactor Parser"}},
	}
	ts := listWorkspacesToolsOn(t, "ws-a", byWS)

	out, err := ts.Call(context.Background(), "list_workspaces", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.Contains(t, out, "c1")
	require.Contains(t, out, "Fix Auth Bug")
	require.Contains(t, out, "c2")
	require.Contains(t, out, "Refactor Parser")
}

// Visibility is downward only: a caller on ws-a1 must never see ws-a (its
// parent) or repo-default (its grandparent), only ws-a1 itself.
func TestListWorkspaces_NeverListsAnAncestor(t *testing.T) {
	ts := listWorkspacesToolsOn(t, "ws-a1", nil)

	out, err := ts.Call(context.Background(), "list_workspaces", json.RawMessage(`{}`))
	require.NoError(t, err)

	headers := workspaceHeaders(out)
	require.ElementsMatch(t, []string{"* ws-a1"}, headers)
	require.NotContains(t, headers, "ws-a")
	require.NotContains(t, headers, "* ws-a")
	require.NotContains(t, headers, "repo-default")
}

type stubChatLogs struct {
	log  string
	read []string
}

func (s *stubChatLogs) ReadChatLog(_ context.Context, chatID string) (string, error) {
	s.read = append(s.read, chatID)
	return s.log, nil
}

// chatLogToolsOn builds a ToolSet on ws-a whose ChatReader resolves the named
// chat into the given workspace.
func chatLogToolsOn(
	t *testing.T,
	target domain.AgentChat,
	logs *stubChatLogs,
) *agenttools.ToolSet {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	chats := stubChats{c: target}
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		chats, stubWorkspaces{all: tree()})
	return agenttools.NewToolSet(agenttools.Deps{
		Resolver: res, ChatReads: chats, ChatLogs: logs,
	}, "RUN", m.Mint("RUN"))
}

func TestGetChatLog_ReturnsTheLedgerRendering(t *testing.T) {
	logs := &stubChatLogs{log: "user: hello\n\nassistant (claude): hi\n"}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "assistant (claude): hi")
	require.Equal(t, []string{"other"}, logs.read)
}

// A chat id is not an authorization: the chat's workspace must be visible.
// ws-b is a sibling, so it is not.
func TestGetChatLog_RejectsAChatOutsideTheCallersScope(t *testing.T) {
	logs := &stubChatLogs{log: "secret"}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-b"}, logs)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, logs.read, "an out-of-scope chat log must never be read from disk")
}

func TestGetChatLog_RejectsAChatOnAnAncestorWorkspace(t *testing.T) {
	logs := &stubChatLogs{log: "secret"}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "repo-default"}, logs)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, logs.read)
}

// An empty ledger is a normal state — a chat that has not spoken yet — and must
// read as such rather than as a failure the model tries to work around.
func TestGetChatLog_EmptyLedgerIsExplicitNotAnError(t *testing.T) {
	logs := &stubChatLogs{log: ""}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-a"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "no turns")
}
