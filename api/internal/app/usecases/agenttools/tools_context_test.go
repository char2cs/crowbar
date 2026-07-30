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

// stubChatsByWorkspace holds the fixture keyed by owning workspace and hands
// list_workspaces the flat whole-table read the real store gives it, stamping
// each chat with the workspace its fixture key names. Keying the fixture by
// workspace is what lets a test prove chats are bucketed to the right workspace
// — and that chats of a workspace the caller cannot see are dropped rather than
// rendered.
//
// calls counts the reads: the point of A3 is that a caller seeing V workspaces
// makes ONE, so a count is the only assertion that can fail if the loop ever
// comes back.
type stubChatsByWorkspace struct {
	byWS  map[string][]domain.AgentChat
	calls int
}

func (s *stubChatsByWorkspace) Get(context.Context, string) (domain.AgentChat, error) {
	return domain.AgentChat{}, nil
}

func (s *stubChatsByWorkspace) ListChats(
	_ context.Context,
) ([]domain.AgentChat, error) {
	s.calls++
	var out []domain.AgentChat
	for wsID, chats := range s.byWS {
		for _, chat := range chats {
			chat.WorkspaceID = wsID
			out = append(out, chat)
		}
	}
	return out, nil
}

func listWorkspacesToolsOn(
	t *testing.T,
	callerWs string,
	byWS map[string][]domain.AgentChat,
) (*agenttools.ToolSet, *stubChatsByWorkspace) {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: domain.AgentRunner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.AgentChat{ID: "CHAT", WorkspaceID: callerWs}},
		stubWorkspaces{all: tree()})
	chats := &stubChatsByWorkspace{byWS: byWS}
	return agenttools.NewToolSet(agenttools.Deps{
		Resolver: res, ChatReads: chats,
	}, "RUN", m.Mint("RUN")), chats
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
	ts, _ := listWorkspacesToolsOn(t, "ws-a", nil)

	out, err := ts.Call(context.Background(), "list_workspaces", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.ElementsMatch(t, []string{"* ws-a", "ws-a1"}, workspaceHeaders(out))
}

func TestListWorkspaces_IncludesEachWorkspacesChats(t *testing.T) {
	byWS := map[string][]domain.AgentChat{
		"ws-a":  {{ID: "c1", Title: "Fix Auth Bug"}},
		"ws-a1": {{ID: "c2", Title: "Refactor Parser"}},
	}
	ts, _ := listWorkspacesToolsOn(t, "ws-a", byWS)

	out, err := ts.Call(context.Background(), "list_workspaces", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.Contains(t, out, "c1")
	require.Contains(t, out, "Fix Auth Bug")
	require.Contains(t, out, "c2")
	require.Contains(t, out, "Refactor Parser")
}

// The chat table is read ONCE however many workspaces the caller can see: the
// store's per-workspace read is a full scan filtered in Go, so a read per
// visible workspace decoded the whole table V times over. ws-a sees itself and
// ws-a1, so a per-workspace loop would show up here as 2.
func TestListWorkspaces_ReadsTheChatTableOnce(t *testing.T) {
	byWS := map[string][]domain.AgentChat{
		"ws-a":  {{ID: "c1", Title: "Fix Auth Bug"}},
		"ws-a1": {{ID: "c2", Title: "Refactor Parser"}},
	}
	ts, chats := listWorkspacesToolsOn(t, "ws-a", byWS)

	_, err := ts.Call(context.Background(), "list_workspaces", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.Equal(t, 1, chats.calls)
}

// Reading the WHOLE chat table and bucketing in Go must not widen what the tool
// renders: a sibling's chats are now in the slice the tool holds, and only the
// visibility filter keeps them out of the output. ws-b is a sibling of ws-a, so
// neither its header nor its chat may appear.
func TestListWorkspaces_DropsChatsOfWorkspacesTheCallerCannotSee(t *testing.T) {
	byWS := map[string][]domain.AgentChat{
		"ws-a": {{ID: "c1", Title: "Fix Auth Bug"}},
		"ws-b": {{ID: "secret-chat", Title: "Siblings Private Work"}},
	}
	ts, _ := listWorkspacesToolsOn(t, "ws-a", byWS)

	out, err := ts.Call(context.Background(), "list_workspaces", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.Contains(t, out, "c1")
	require.NotContains(t, out, "secret-chat")
	require.NotContains(t, out, "Siblings Private Work")
}

// Visibility is downward only: a caller on ws-a1 must never see ws-a (its
// parent) or repo-default (its grandparent), only ws-a1 itself.
func TestListWorkspaces_NeverListsAnAncestor(t *testing.T) {
	ts, _ := listWorkspacesToolsOn(t, "ws-a1", nil)

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
