package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestBoundedTools_TheCapsAreTheNumbersTheBudgetWasComputedFrom(t *testing.T) {
	require.Equal(t, 20, tools.DefaultThreadPageForTest)
	require.Equal(t, 50, tools.MaxThreadPageForTest)
	require.Equal(t, 4, tools.MaxThreadMessagesForTest)
	require.Equal(t, 20, tools.DefaultChatLogTurnsForTest)
	require.Equal(t, 50, tools.MaxChatLogTurnsForTest)
	require.Equal(t, 100, tools.DefaultScopeFilesForTest)
	require.Equal(t, 300, tools.MaxScopeFilesForTest)
	require.Equal(t, 6, tools.MaxScopeRangesPerFileForTest)
	require.Equal(t, 300, tools.MaxScopeRangesForTest)
	require.Equal(t, 200, tools.MaxMessageBodyCharsForTest)
	require.Equal(t, 800, tools.MaxTurnBodyCharsForTest)
}

func TestBoundedTools_AnUntruncatedResultStillStatesItsTotal(t *testing.T) {
	threads := &stubThreadReader{list: manyThreads(2)}
	review := &stubReviewReader{base: "abc123", files: manyFiles(3)}
	ts, _ := reviewToolsetOn(t, threads, review)

	out, err := ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Contains(t, out, "Showing all 2 threads.")

	out, err = ts.Call(context.Background(), "get_review_scope", json.RawMessage(`{}`))
	require.NoError(t, err)
	require.Contains(t, out, "Showing all 3 changed files.")

	logs := &stubChatLogs{turns: chatTurns(4)}
	chatTS := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-a1"}, logs)
	out, err = chatTS.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)
	require.Contains(t, out, "Showing all 4 turns, oldest first.")
}

func TestBoundedTools_PaginationCannotReachPastTheCallersScope(t *testing.T) {
	threads := &stubThreadReader{list: manyThreads(2)}
	review := &stubReviewReader{base: "abc", files: manyFiles(2)}
	ts, _ := reviewToolsetOn(t, threads, review)

	_, err := ts.Call(context.Background(), "list_review_threads",
		json.RawMessage(`{"offset":100000,"limit":100000}`))
	require.NoError(t, err)
	require.Equal(t, "ws-a", threads.lastWsID)

	_, err = ts.Call(context.Background(), "get_review_scope",
		json.RawMessage(`{"offset":100000,"limit":100000}`))
	require.NoError(t, err)
	require.Equal(t, "ws-a", review.lastWsID)

	logs := &stubChatLogs{turns: chatTurns(5)}
	chatTS := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-b"}, logs)
	_, err = chatTS.Call(context.Background(), "get_chat_log",
		json.RawMessage(`{"chatId":"other","offset":0,"limit":100000}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, logs.read, "an out-of-scope chat log must never be read, paged or not")
}

func TestBoundedTools_AdvertiseOffsetAndLimitAndNoOutputSchema(t *testing.T) {
	ts, _ := toolsetOn(t, &spyRenamer{})
	bounded := map[string]bool{
		"list_review_threads": false,
		"get_review_scope":    false,
		"get_chat_log":        false,
	}
	for _, tool := range ts.Tools() {
		require.NotContains(t, string(tool.InputSchema), "outputSchema",
			"tool %s declares an outputSchema", tool.Name)
		if _, ok := bounded[tool.Name]; !ok {
			continue
		}
		require.Contains(t, string(tool.InputSchema), `"offset"`, "%s cannot be paged", tool.Name)
		require.Contains(t, string(tool.InputSchema), `"limit"`, "%s cannot be bounded", tool.Name)
		bounded[tool.Name] = true
	}
	for name, seen := range bounded {
		require.True(t, seen, "%s is not registered, so this test proved nothing about it", name)
	}
}
