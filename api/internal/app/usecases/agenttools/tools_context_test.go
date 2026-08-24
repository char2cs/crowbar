package agenttools_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

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
	byWS  map[string][]domain.Chat
	calls int
}

func (s *stubChatsByWorkspace) Get(context.Context, string) (domain.Chat, error) {
	return domain.Chat{}, nil
}

func (s *stubChatsByWorkspace) ListChats(
	_ context.Context,
) ([]domain.Chat, error) {
	s.calls++
	var out []domain.Chat
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
	byWS map[string][]domain.Chat,
) (*agenttools.ToolSet, *stubChatsByWorkspace) {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	res := agenttools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: callerWs}},
		stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: callerWs}},
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
	byWS := map[string][]domain.Chat{
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
	byWS := map[string][]domain.Chat{
		"ws-a":  {{ID: "c1", Title: "Fix Auth Bug"}},
		"ws-a1": {{ID: "c2", Title: "Refactor Parser"}},
	}
	ts, chats := listWorkspacesToolsOn(t, "ws-a", byWS)

	_, err := ts.Call(context.Background(), "list_workspaces", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.Equal(t, 1, chats.calls)
}

// chatsByWorkspace's membership filter is asserted HERE, on the map it returns,
// because it is unobservable from list_workspaces' rendered text:
// renderWorkspaces walks the visible slice and looks each id up, so a bucket for
// an invisible workspace is simply never reached and the output is byte-
// identical with the filter deleted. The tool-level test below was written as
// this filter's guard and passed with the filter removed; the filter is defence
// in depth behind renderWorkspaces, and defence in depth still has to be
// something a test can break.
//
// The expected map is asserted whole, not probed key by key: an extra "ws-b"
// bucket is the failure being guarded against, and only an exact comparison
// notices a key that should not exist. The nil "ws-a1" entry is the other half —
// every visible workspace is seeded so one with no chats still renders as itself
// rather than vanishing.
func TestChatsByWorkspace_DropsChatsOfWorkspacesTheCallerCannotSee(t *testing.T) {
	all := []domain.Chat{
		{ID: "c1", WorkspaceID: "ws-a", Title: "Fix Auth Bug"},
		{ID: "secret-chat", WorkspaceID: "ws-b", Title: "Siblings Private Work"},
	}
	visible := []domain.Workspace{{ID: "ws-a"}, {ID: "ws-a1"}}

	got := agenttools.ChatsByWorkspaceForTest(all, visible)

	require.Equal(t, map[string][]domain.Chat{
		"ws-a":  {{ID: "c1", WorkspaceID: "ws-a", Title: "Fix Auth Bug"}},
		"ws-a1": nil,
	}, got, "a chat whose workspace is not visible must not be bucketed at all")
}

// This is the OUTER boundary — renderWorkspaces iterating the visible set — not
// the bucketing filter, which is guarded by the test above. Both layers matter:
// this one is what actually keeps a sibling's chats out of the bytes an agent
// reads, and it must keep holding even if the bucketing ever changes shape.
// ws-b is a sibling of ws-a, so neither its header nor its chat may appear.
func TestListWorkspaces_DropsChatsOfWorkspacesTheCallerCannotSee(t *testing.T) {
	byWS := map[string][]domain.Chat{
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
	turns []agenttools.ChatTurn
	read  []string
}

func (s *stubChatLogs) ReadChatLog(
	_ context.Context,
	chatID string,
) ([]agenttools.ChatTurn, error) {
	s.read = append(s.read, chatID)
	return s.turns, nil
}

// chatTurns builds n turns whose bodies carry their own 1-based number, so a
// test can name the exact turn it expects at each end of a window instead of
// merely counting lines.
func chatTurns(n int) []agenttools.ChatTurn {
	out := make([]agenttools.ChatTurn, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, agenttools.ChatTurn{
			Speaker: "user",
			Body:    fmt.Sprintf("turn-%d", i),
		})
	}
	return out
}

// stubLineage is an agenttools.ChatLineageReader whose answer is fixed per chat
// id. An id with no entry has no chat ancestors, which is what nearly every chat
// in these fixtures is: unthreaded, at the panel root.
//
// It records what it was ASKED about, not merely how often. get_chat_log has to
// resolve the TARGET's lineage rather than the caller's — a caller is an
// ancestor exactly when the target's chain names it — and a stub that only
// counted calls would pass just as happily on the walk taken from the wrong end.
type stubLineage struct {
	byChat map[string][]string
	err    error
	asked  []string
}

func (s *stubLineage) Ancestors(
	_ context.Context,
	chatID string,
) ([]string, error) {
	s.asked = append(s.asked, chatID)
	if s.err != nil {
		return nil, s.err
	}
	return s.byChat[chatID], nil
}

// chatLogToolsOn builds a ToolSet on ws-a whose ChatReader resolves the named
// chat into the given workspace, and whose target chat is threaded off nobody.
func chatLogToolsOn(
	t *testing.T,
	target domain.Chat,
	logs *stubChatLogs,
) *agenttools.ToolSet {
	t.Helper()
	return chatLogToolsUnder(t, target, logs, &stubLineage{})
}

// chatLogToolsUnder is chatLogToolsOn with the Chats-panel tree spelled out, for
// the tests that turn on where the target sits relative to the caller.
func chatLogToolsUnder(
	t *testing.T,
	target domain.Chat,
	logs *stubChatLogs,
	lineage *stubLineage,
) *agenttools.ToolSet {
	t.Helper()
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	chats := stubChats{c: target}
	res := agenttools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		chats, stubWorkspaces{all: tree()})
	return agenttools.NewToolSet(agenttools.Deps{
		Resolver: res, ChatReads: chats, ChatLogs: logs, Lineage: lineage,
	}, "RUN", m.Mint("RUN"))
}

func TestGetChatLog_ReturnsTheLedgerRendering(t *testing.T) {
	logs := &stubChatLogs{turns: []agenttools.ChatTurn{
		{Speaker: "user", Body: "hello"},
		{Speaker: "assistant (claude)", Body: "hi"},
	}}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "assistant (claude): hi")
	require.Equal(t, []string{"other"}, logs.read)
}

// A chat id is not an authorization: the chat's workspace must be visible.
// ws-b is a sibling, so it is not.
func TestGetChatLog_RejectsAChatOutsideTheCallersScope(t *testing.T) {
	logs := &stubChatLogs{turns: []agenttools.ChatTurn{{Speaker: "user", Body: "secret"}}}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-b"}, logs)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, logs.read, "an out-of-scope chat log must never be read from disk")
}

func TestGetChatLog_RejectsAChatOnAnAncestorWorkspace(t *testing.T) {
	logs := &stubChatLogs{turns: []agenttools.ChatTurn{{Speaker: "user", Body: "secret"}}}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "repo-default"}, logs)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, logs.read)
}

// An empty ledger is a normal state — a chat that has not spoken yet — and must
// read as such rather than as a failure the model tries to work around.
func TestGetChatLog_EmptyLedgerIsExplicitNotAnError(t *testing.T) {
	logs := &stubChatLogs{}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "no turns")
}

// ---------------------------------------------------------------------------
// get_chat_log: the one direction that is closed
// ---------------------------------------------------------------------------

// A thread reads the chat it hangs off. This is the permission the whole feature
// rests on, and it is asserted rather than assumed: threads live in the same
// workspace as their parent, so workspace scope was always expected to admit it
// — but "expected to" is not a test, and the descendant refusal added beside it
// is exactly the kind of check that overshoots.
func TestGetChatLog_AThreadReadsTheChatItHangsOff(t *testing.T) {
	logs := &stubChatLogs{turns: []agenttools.ChatTurn{{Speaker: "user", Body: "the parent's plan"}}}
	// CHAT is the caller. Its own lineage names PARENT; PARENT's names nobody.
	lineage := &stubLineage{byChat: map[string][]string{"CHAT": {"PARENT"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "PARENT", WorkspaceID: "ws-a"}, logs, lineage)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"PARENT"}`))
	require.NoError(t, err)
	require.Contains(t, out, "the parent's plan")
	require.Equal(t, []string{"PARENT"}, lineage.asked,
		"the refusal is decided from the TARGET's chain, since a caller is an ancestor exactly when the target names it")
}

// The refusal. A parent must not read its own threads: it is never handed them,
// and it may not go and fetch them either — otherwise three threads off one chat
// stop being three independent attempts.
func TestGetChatLog_AChatCannotReadItsOwnThread(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(3)}
	lineage := &stubLineage{byChat: map[string][]string{"THREAD": {"CHAT"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "THREAD", WorkspaceID: "ws-a"}, logs, lineage)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"THREAD"}`))
	require.ErrorIs(t, err, agenttools.ErrOwnThread)
	require.Empty(t, logs.read, "a refused read must never reach the ledger on disk")
}

// The refusal reaches all the way down, not just one level: a thread of a thread
// is still this chat's descendant.
func TestGetChatLog_AChatCannotReadAThreadOfItsOwnThread(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(3)}
	lineage := &stubLineage{byChat: map[string][]string{"DEEP": {"THREAD", "CHAT"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "DEEP", WorkspaceID: "ws-a"}, logs, lineage)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"DEEP"}`))
	require.ErrorIs(t, err, agenttools.ErrOwnThread)
}

// And it survives the folders a user files a thread into, because the lineage it
// is decided from steps straight through them. A check written against a stored
// parent id would have missed every filed thread, which is the same as not
// having a check.
func TestGetChatLog_AFiledThreadIsStillThisChatsThread(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(3)}
	// THREAD sits in a folder in a folder under CHAT; folders never appear in a
	// lineage, so what the walk hands back is the caller and nothing else.
	lineage := &stubLineage{byChat: map[string][]string{"THREAD": {"CHAT"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "THREAD", WorkspaceID: "ws-a"}, logs, lineage)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"THREAD"}`))
	require.ErrorIs(t, err, agenttools.ErrOwnThread)
}

// Siblings read each other, and that is DELIBERATE — it is what this tool was
// built for, and it predates threads entirely. Two threads off one chat are two
// agents that may compare notes; only the direction DOWN from a chat into its
// own threads is closed.
func TestGetChatLog_SiblingThreadsStillReadEachOther(t *testing.T) {
	logs := &stubChatLogs{turns: []agenttools.ChatTurn{{Speaker: "user", Body: "the other attempt"}}}
	// CHAT and SIBLING are both threads of PARENT. Neither is below the other.
	lineage := &stubLineage{byChat: map[string][]string{
		"CHAT":    {"PARENT"},
		"SIBLING": {"PARENT"},
	}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "SIBLING", WorkspaceID: "ws-a"}, logs, lineage)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"SIBLING"}`))
	require.NoError(t, err)
	require.Contains(t, out, "the other attempt")
}

// An unrelated chat in a workspace the caller can see stays readable too: the
// new check narrows the tool to one case and must not become a general "only
// your ancestors" rule.
func TestGetChatLog_AnUnrelatedChatInScopeStaysReadable(t *testing.T) {
	logs := &stubChatLogs{turns: []agenttools.ChatTurn{{Speaker: "user", Body: "somebody else's work"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"},
		logs, &stubLineage{})

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "somebody else's work")
}

// A tree the daemon cannot read is not an empty tree. Serving the log anyway
// would open the one closed direction at the exact moment nothing could tell a
// thread from a sibling.
func TestGetChatLog_ALineageThatCannotBeReadRefuses(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(2)}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "other", WorkspaceID: "ws-a"},
		logs, &stubLineage{err: errors.New("tree unreadable")})

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, agenttools.ErrOwnThread)
	require.ErrorContains(t, err, "tree unreadable")
	require.Empty(t, logs.read)
}

// The scope check runs FIRST, so a caller that was never allowed to see the
// workspace is turned away without the tree being read at all.
func TestGetChatLog_AnOutOfScopeChatIsRefusedBeforeTheLineageIsRead(t *testing.T) {
	lineage := &stubLineage{}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "other", WorkspaceID: "ws-b"},
		&stubChatLogs{}, lineage)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
	require.Empty(t, lineage.asked)
}

// The port is an authority, so its absence withdraws the tool rather than
// serving it with the refusal silently switched off.
func TestGetChatLog_IsNotAdvertisedWithoutTheLineagePort(t *testing.T) {
	m, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	chats := stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}}
	res := agenttools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}},
		chats, stubWorkspaces{all: tree()})
	ts := agenttools.NewToolSet(agenttools.Deps{
		Resolver: res, ChatReads: chats, ChatLogs: &stubChatLogs{},
	}, "RUN", m.Mint("RUN"))

	names := []string{}
	for _, tool := range ts.Tools() {
		names = append(names, tool.Name)
	}
	require.NotContains(t, names, "get_chat_log")
	require.Contains(t, names, "list_workspaces",
		"the sibling tool has its own dependency and must not be withdrawn with this one")
}
