package tools_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// The caps are NUMBERS, and every test below sizes its fixture from them — which
// is what keeps those tests meaningful when a cap moves, and also exactly what
// makes them blind to it moving: a fixture of "the cap plus five" still renders
// "the cap" rows whatever the cap became. Proven by mutation — with the caps
// raised to 100000 every scaled test below still passed.
//
// So the numbers are pinned here, once. Raising a cap then has to be a
// deliberate edit against limits.go's stated token budget rather than a silent
// change that no test notices.
func TestBoundedTools_TheCapsAreTheNumbersTheBudgetWasComputedFrom(t *testing.T) {
	require.Equal(t, 20, tools.DefaultThreadPageForTest)
	require.Equal(t, 50, tools.MaxThreadPageForTest)
	require.Equal(t, 4, tools.MaxThreadMessagesForTest)
	require.Equal(t, 20, tools.DefaultChatLogTurnsForTest)
	require.Equal(t, 50, tools.MaxChatLogTurnsForTest)
	require.Equal(t, 100, tools.DefaultScopeFilesForTest)
	require.Equal(t, 300, tools.MaxScopeFilesForTest)
	// The geometry caps are the same budget again, on the one part of a scope
	// reply whose size follows the DIFF rather than the file count: ~12 characters
	// a range, so 300 of them is ~3.6 KB — about what the file rows themselves
	// cost. Six per file per side is a file's working set; past that the file is
	// one to open rather than to anchor from a listing.
	require.Equal(t, 6, tools.MaxScopeRangesPerFileForTest)
	require.Equal(t, 300, tools.MaxScopeRangesForTest)
	// The body caps are the same 16 KB page budget divided by each tool's
	// worst-case body count: 16384/(20 threads × 4 messages) for a review message,
	// 16384/(20 turns) for a chat turn. Raising either without re-deriving it is
	// how a "bounded" page silently becomes a 20k-token one again.
	require.Equal(t, 200, tools.MaxMessageBodyCharsForTest)
	require.Equal(t, 800, tools.MaxTurnBodyCharsForTest)
}

// A short list still states its total. "Showing all 2 threads" and a bare list
// of two threads are the same bytes to a model only if it can already tell the
// list is complete — and it cannot, which is the entire reason the note is
// unconditional.
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

// Pagination must not become a scope argument by the back door: offset and limit
// choose a window of what the caller could ALREADY see, and no value of either
// changes which workspace is read or gets a rejected chat read from disk.
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

	// ws-b is a sibling of the caller's ws-a. Paging arguments must not change
	// that the read is refused before ChatLogs is ever touched.
	logs := &stubChatLogs{turns: chatTurns(5)}
	chatTS := chatLogToolsOn(t, domain.Chat{ID: "other", WorkspaceID: "ws-b"}, logs)
	_, err = chatTS.Call(context.Background(), "get_chat_log",
		json.RawMessage(`{"chatId":"other","offset":0,"limit":100000}`))
	require.ErrorIs(t, err, tools.ErrOutOfScope)
	require.Empty(t, logs.read, "an out-of-scope chat log must never be read, paged or not")
}

// Every bounded tool has to ADVERTISE its paging, or a model has no way to
// discover the arguments the truncation note tells it to use. And none of them
// may declare an outputSchema — a global constraint of this surface.
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
