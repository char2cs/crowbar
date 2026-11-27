package tools_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

func TestGetChatLog_CapsATurnBodyAndSaysHowMuchItCut(t *testing.T) {
	over := 42
	body := longBody(tools.MaxTurnBodyCharsForTest + over)
	logs := &stubChatLogs{turns: []tools.ChatTurn{{Speaker: "assistant (claude)", Body: body}}}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)

	require.NotContains(t, out, body)
	require.Contains(t, out,
		longBody(tools.MaxTurnBodyCharsForTest)+
			fmt.Sprintf("… [shortened; %d characters not shown]", over))
}

func TestGetChatLog_KeepsTheMostRecentTurnsAndSaysWhatItDropped(t *testing.T) {
	total := 63
	logs := &stubChatLogs{turns: chatTurns(total)}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)

	kept := tools.DefaultChatLogTurnsForTest
	require.Contains(t, out, fmt.Sprintf(
		"Showing turns %d-%d of %d, oldest first. %d older turns not shown — call get_chat_log with offset=%d for the %d before these.",
		total-kept+1, total, total, total-kept, kept, kept,
	))
	require.Contains(t, out, fmt.Sprintf("turn-%d", total))
	require.Contains(t, out, fmt.Sprintf("turn-%d", total-kept+1))
	require.NotContains(t, out, "turn-1\n", "the oldest turns are the ones the cap drops")
}

func TestGetChatLog_PagesBackwardsIntoOlderHistory(t *testing.T) {
	total := 63
	kept := tools.DefaultChatLogTurnsForTest
	logs := &stubChatLogs{turns: chatTurns(total)}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log",
		json.RawMessage(fmt.Sprintf(`{"chatId":"other","offset":%d}`, kept)))
	require.NoError(t, err)

	require.Contains(t, out, fmt.Sprintf("turn-%d\n", total-kept))
	require.Contains(t, out, fmt.Sprintf("turn-%d\n", total-(2*kept)+1))
	require.NotContains(t, out, fmt.Sprintf("turn-%d\n", total),
		"a backwards page must not re-render the turns the previous page already showed")
}

func TestGetChatLog_ReachingTheStartSaysNothingIsOlder(t *testing.T) {
	total := 25
	logs := &stubChatLogs{turns: chatTurns(total)}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log",
		json.RawMessage(fmt.Sprintf(`{"chatId":"other","offset":%d}`, total-5)))
	require.NoError(t, err)

	require.Contains(t, out, fmt.Sprintf("Showing turns 1-5 of %d, oldest first. Nothing older exists.", total))
	require.Contains(t, out, "turn-1\n")
	require.NotContains(t, out, "turn-6\n", "the window still ends where the offset put it")
}

func TestGetChatLog_ClampsAnOversizedLimit(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(tools.MaxChatLogTurnsForTest + 40)}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log",
		json.RawMessage(`{"chatId":"other","limit":99999}`))
	require.NoError(t, err)

	require.Equal(t, tools.MaxChatLogTurnsForTest,
		strings.Count(out, "user: turn-"))
}

func TestGetChatLog_ANegativeOffsetReadsAsTheNewestPage(t *testing.T) {
	total := 5
	logs := &stubChatLogs{turns: chatTurns(total)}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log",
		json.RawMessage(`{"chatId":"other","offset":-1}`))
	require.NoError(t, err)

	require.Contains(t, out, fmt.Sprintf("Showing all %d turns, oldest first.", total))
	require.Contains(t, out, fmt.Sprintf("turn-%d\n", total),
		"a negative offset must not page past the newest turn")
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

func TestChatsByWorkspace_DropsChatsOfWorkspacesTheCallerCannotSee(t *testing.T) {
	all := []domain.Chat{
		{ID: "c1", WorkspaceID: "ws-a", Title: "Fix Auth Bug"},
		{ID: "secret-chat", WorkspaceID: "ws-b", Title: "Siblings Private Work"},
	}
	visible := []domain.Workspace{{ID: "ws-a"}, {ID: "ws-a1"}}

	got := tools.ChatsByWorkspaceForTest(all, visible)

	require.Equal(t, map[string][]domain.Chat{
		"ws-a":  {{ID: "c1", WorkspaceID: "ws-a", Title: "Fix Auth Bug"}},
		"ws-a1": nil,
	}, got, "a chat whose workspace is not visible must not be bucketed at all")
}

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

func TestGetChatLog_ReturnsTheLedgerRendering(t *testing.T) {
	logs := &stubChatLogs{turns: []tools.ChatTurn{
		{Speaker: "user", Body: "hello"},
		{Speaker: "assistant (claude)", Body: "hi"},
	}}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "assistant (claude): hi")
	require.Equal(t, []string{"other"}, logs.read)
}

func TestGetChatLog_RejectsAChatOutsideTheCallersScope(t *testing.T) {
	logs := &stubChatLogs{turns: []tools.ChatTurn{{Speaker: "user", Body: "secret"}}}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-b"}, logs)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, logs.read, "an out-of-scope chat log must never be read from disk")
}

func TestGetChatLog_RejectsAChatOnAnAncestorWorkspace(t *testing.T) {
	logs := &stubChatLogs{turns: []tools.ChatTurn{{Speaker: "user", Body: "secret"}}}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "repo-default"}, logs)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, logs.read)
}

func TestGetChatLog_EmptyLedgerIsExplicitNotAnError(t *testing.T) {
	logs := &stubChatLogs{}
	ts := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "no turns")
}

func TestGetChatLog_AThreadReadsTheChatItHangsOff(t *testing.T) {
	logs := &stubChatLogs{turns: []tools.ChatTurn{{Speaker: "user", Body: "the parent's plan"}}}
	lineage := &stubLineage{byChat: map[string][]string{"CHAT": {"PARENT"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "PARENT", WorkspaceID: "ws-a"}, logs, lineage)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"PARENT"}`))
	require.NoError(t, err)
	require.Contains(t, out, "the parent's plan")
	require.Equal(t, []string{"PARENT"}, lineage.asked,
		"the refusal is decided from the TARGET's chain, since a caller is an ancestor exactly when the target names it")
}

func TestGetChatLog_AChatCannotReadItsOwnThread(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(3)}
	lineage := &stubLineage{byChat: map[string][]string{"THREAD": {"CHAT"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "THREAD", WorkspaceID: "ws-a"}, logs, lineage)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"THREAD"}`))
	require.ErrorIs(t, err, tools.ErrOwnThread)
	require.Empty(t, logs.read, "a refused read must never reach the ledger on disk")
}

func TestGetChatLog_AChatCannotReadAThreadOfItsOwnThread(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(3)}
	lineage := &stubLineage{byChat: map[string][]string{"DEEP": {"THREAD", "CHAT"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "DEEP", WorkspaceID: "ws-a"}, logs, lineage)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"DEEP"}`))
	require.ErrorIs(t, err, tools.ErrOwnThread)
}

func TestGetChatLog_AFiledThreadIsStillThisChatsThread(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(3)}
	lineage := &stubLineage{byChat: map[string][]string{"THREAD": {"CHAT"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "THREAD", WorkspaceID: "ws-a"}, logs, lineage)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"THREAD"}`))
	require.ErrorIs(t, err, tools.ErrOwnThread)
}

func TestGetChatLog_SiblingThreadsStillReadEachOther(t *testing.T) {
	logs := &stubChatLogs{turns: []tools.ChatTurn{{Speaker: "user", Body: "the other attempt"}}}
	lineage := &stubLineage{byChat: map[string][]string{
		"CHAT":    {"PARENT"},
		"SIBLING": {"PARENT"},
	}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "SIBLING", WorkspaceID: "ws-a"}, logs, lineage)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"SIBLING"}`))
	require.NoError(t, err)
	require.Contains(t, out, "the other attempt")
}

func TestGetChatLog_AnUnrelatedChatInScopeStaysReadable(t *testing.T) {
	logs := &stubChatLogs{turns: []tools.ChatTurn{{Speaker: "user", Body: "somebody else's work"}}}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"},
		logs, &stubLineage{})

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "somebody else's work")
}

func TestGetChatLog_ALineageThatCannotBeReadRefuses(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(2)}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "other", WorkspaceID: "ws-a"},
		logs, &stubLineage{err: errors.New("tree unreadable")})

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, tools.ErrOwnThread)
	require.ErrorContains(t, err, "tree unreadable")
	require.Empty(t, logs.read)
}

func TestGetChatLog_AnOutOfScopeChatIsRefusedBeforeTheLineageIsRead(t *testing.T) {
	lineage := &stubLineage{}
	ts := chatLogToolsUnder(t, domain.Chat{ID: "other", WorkspaceID: "ws-b"},
		&stubChatLogs{}, lineage)

	_, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, lineage.asked)
}

func TestGetChatLog_IsNotAdvertisedWithoutTheLineagePort(t *testing.T) {
	m, err := tools.NewTokenMinter()
	require.NoError(t, err)
	chats := stubChats{c: domain.Chat{ID: "CHAT", WorkspaceID: "ws-a"}}
	res := tools.NewResolver(m,
		stubRunners{r: agents.Runner{ID: "RUN", CurrentChatID: "CHAT", WorkspaceID: "ws-a"}}, stubWorkspaces{all: tree()})
	ts := tools.NewToolSet(tools.Deps{
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
