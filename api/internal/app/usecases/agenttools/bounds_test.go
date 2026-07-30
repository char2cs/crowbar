package agenttools_test

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/agenttools"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// manyThreads builds n unresolved threads with ids "t-1".."t-n", so a test can
// name the exact thread it expects at each edge of a page rather than counting
// rows and hoping the count lines up with the right ones.
func manyThreads(n int) []domain.ReviewThread {
	out := make([]domain.ReviewThread, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, domain.ReviewThread{
			ID: fmt.Sprintf("t-%d", i), WsID: "ws-a",
			FilePath: fmt.Sprintf("f%d.go", i), StartLine: 1, EndLine: 1,
			Side: domain.ReviewSideRight, Status: domain.ReviewThreadStatusOpen,
			Messages: []domain.ReviewMessage{{ID: "m1", Author: "mateo", Body: "look at this"}},
		})
	}
	return out
}

func manyFiles(n int) []gitdomain.ReviewFileSummary {
	out := make([]gitdomain.ReviewFileSummary, 0, n)
	for i := 1; i <= n; i++ {
		out = append(out, gitdomain.ReviewFileSummary{
			Path:      fmt.Sprintf("src/f%d.go", i),
			Status:    gitdomain.GitFileStatusModified,
			Additions: 1, Deletions: 1,
		})
	}
	return out
}

// threadRows counts rendered thread anchor rows: the unindented lines that are
// neither the pagination note nor the column header. Counting rows is what
// proves the CAP, as opposed to merely proving that some particular thread is
// missing.
func threadRows(out string) int {
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		switch {
		case line == "", strings.HasPrefix(line, " "):
		case strings.HasPrefix(line, "Showing"), strings.HasPrefix(line, "No "):
		case strings.HasPrefix(line, "id  file:lines"):
		default:
			n++
		}
	}
	return n
}

func fileRows(out string) int {
	n := 0
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.HasPrefix(line, string(gitdomain.GitFileStatusModified)+"  +") {
			n++
		}
	}
	return n
}

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
	require.Equal(t, 20, agenttools.DefaultThreadPageForTest)
	require.Equal(t, 50, agenttools.MaxThreadPageForTest)
	require.Equal(t, 4, agenttools.MaxThreadMessagesForTest)
	require.Equal(t, 20, agenttools.DefaultChatLogTurnsForTest)
	require.Equal(t, 50, agenttools.MaxChatLogTurnsForTest)
	require.Equal(t, 100, agenttools.DefaultScopeFilesForTest)
	require.Equal(t, 300, agenttools.MaxScopeFilesForTest)
}

// A branch with more threads than one page renders one page — and says so. The
// note is asserted whole, not by keyword: it is the entire mechanism by which a
// model learns the list is partial, and a note that lost its offset would still
// contain the word "Showing".
func TestListReviewThreads_PagesTheThreadListAndSaysWhatIsMissing(t *testing.T) {
	over := agenttools.DefaultThreadPageForTest + 5
	stub := &stubThreadReader{list: manyThreads(over)}
	ts, _ := reviewToolsetOn(t, stub, &stubReviewReader{})

	out, err := ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.Equal(t, agenttools.DefaultThreadPageForTest, threadRows(out))
	require.Contains(t, out, fmt.Sprintf(
		"Showing threads 1-%d of %d. 5 not shown — call list_review_threads with offset=%d for the next page.",
		agenttools.DefaultThreadPageForTest, over, agenttools.DefaultThreadPageForTest))
	require.Contains(t, out, "t-1")
	require.NotContains(t, out, fmt.Sprintf("t-%d ", over))
}

// The offset the note handed out has to actually work, and the last page has to
// announce itself: a model told "call with offset=20" that then received another
// "next page" pointer would page forever.
func TestListReviewThreads_TheOffsetTheNoteGivesFetchesTheRest(t *testing.T) {
	over := agenttools.DefaultThreadPageForTest + 5
	stub := &stubThreadReader{list: manyThreads(over)}
	ts, _ := reviewToolsetOn(t, stub, &stubReviewReader{})

	out, err := ts.Call(context.Background(), "list_review_threads",
		json.RawMessage(fmt.Sprintf(`{"offset":%d}`, agenttools.DefaultThreadPageForTest)))
	require.NoError(t, err)

	require.Equal(t, 5, threadRows(out))
	require.Contains(t, out, fmt.Sprintf(
		"Showing threads %d-%d of %d. This is the last page.",
		agenttools.DefaultThreadPageForTest+1, over, over))
	require.Contains(t, out, fmt.Sprintf("t-%d", over))
	require.NotContains(t, out, "next page")
}

// limit is a MODEL-supplied number, so the ceiling is the only thing standing
// between the cap and one argument that undoes it.
func TestListReviewThreads_ClampsAnOversizedLimit(t *testing.T) {
	over := agenttools.MaxThreadPageForTest + 20
	stub := &stubThreadReader{list: manyThreads(over)}
	ts, _ := reviewToolsetOn(t, stub, &stubReviewReader{})

	out, err := ts.Call(context.Background(), "list_review_threads",
		json.RawMessage(`{"limit":100000}`))
	require.NoError(t, err)

	require.Equal(t, agenttools.MaxThreadPageForTest, threadRows(out))
}

// An offset past the last page must not render as "No review threads." — that
// is the answer for a branch with nothing to address, and a model given it after
// over-paging would stop looking while findings were still open.
func TestListReviewThreads_OffsetPastTheEndIsNotAnEmptyReview(t *testing.T) {
	stub := &stubThreadReader{list: manyThreads(3)}
	ts, _ := reviewToolsetOn(t, stub, &stubReviewReader{})

	out, err := ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{"offset":900}`))
	require.NoError(t, err)

	require.NotContains(t, out, "No review threads")
	require.Contains(t, out, "No threads at that offset; there are 3 in total")
}

// One long thread is capped to its root plus the most recent replies, and the
// anchor row keeps reporting the TRUE message count — which is what makes the
// elision checkable rather than merely stated.
func TestListReviewThreads_CapsMessagesPerThreadAndNamesWhatItDropped(t *testing.T) {
	total := agenttools.MaxThreadMessagesForTest + 6
	msgs := make([]domain.ReviewMessage, 0, total)
	for i := 1; i <= total; i++ {
		msgs = append(msgs, domain.ReviewMessage{
			ID: fmt.Sprintf("m%d", i), Author: "mateo", Body: fmt.Sprintf("msg-%d", i),
		})
	}
	stub := &stubThreadReader{list: []domain.ReviewThread{{
		ID: "t1", WsID: "ws-a", FilePath: "a.go", StartLine: 1, EndLine: 1,
		Side: domain.ReviewSideRight, Status: domain.ReviewThreadStatusOpen,
		Messages: msgs,
	}}}
	ts, _ := reviewToolsetOn(t, stub, &stubReviewReader{})

	out, err := ts.Call(context.Background(), "list_review_threads", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.Contains(t, out, fmt.Sprintf("t1  a.go:1-1  right  unresolved  %d", total),
		"the anchor row must report every message, so the elision below is checkable")
	require.Contains(t, out, fmt.Sprintf(
		"... %d middle replies not shown (root + %d most recent below)",
		total-agenttools.MaxThreadMessagesForTest, agenttools.MaxThreadMessagesForTest-1))

	// Each needle carries the whole rendered message row. "msg-1" alone is a
	// substring of "msg-10", so it would match the newest reply and pass even
	// with the root dropped — the exact assertion this test exists to make.
	require.Contains(t, out, "mateo: msg-1\n", "the root IS the finding and must always render")
	require.Contains(t, out, fmt.Sprintf("mateo: msg-%d\n", total),
		"the newest reply is the thread's current state")
	require.NotContains(t, out, "mateo: msg-2\n", "the middle of the thread is what gets dropped")
}

func TestGetReviewScope_CapsTheChangedFileListAndSaysSo(t *testing.T) {
	over := agenttools.DefaultScopeFilesForTest + 50
	stub := &stubReviewReader{base: "abc123", files: manyFiles(over)}
	ts, _ := reviewToolsetOn(t, &stubThreadReader{}, stub)

	out, err := ts.Call(context.Background(), "get_review_scope", json.RawMessage(`{}`))
	require.NoError(t, err)

	require.Equal(t, agenttools.DefaultScopeFilesForTest, fileRows(out))
	require.Contains(t, out, fmt.Sprintf(
		"Showing changed files 1-%d of %d. 50 not shown — call get_review_scope with offset=%d for the next page.",
		agenttools.DefaultScopeFilesForTest, over, agenttools.DefaultScopeFilesForTest))
	// The base ref survives the cap: it is the anchor for every finding, and a
	// paged file list that lost it would describe a diff against nothing.
	require.Contains(t, out, "abc123")
}

// A capped file list that could not be paged past would turn a large diff into a
// review of its first hundred files — post_review_comment rejects a path that is
// not in the review, so an unreachable file is an unwritable finding.
func TestGetReviewScope_PagesPastTheCap(t *testing.T) {
	over := agenttools.DefaultScopeFilesForTest + 50
	stub := &stubReviewReader{base: "abc123", files: manyFiles(over)}
	ts, _ := reviewToolsetOn(t, &stubThreadReader{}, stub)

	out, err := ts.Call(context.Background(), "get_review_scope",
		json.RawMessage(fmt.Sprintf(`{"offset":%d}`, agenttools.DefaultScopeFilesForTest)))
	require.NoError(t, err)

	require.Equal(t, 50, fileRows(out))
	require.Contains(t, out, fmt.Sprintf("src/f%d.go", over))
	require.Contains(t, out, "This is the last page.")
}

func TestGetReviewScope_ClampsAnOversizedLimit(t *testing.T) {
	stub := &stubReviewReader{base: "abc123", files: manyFiles(agenttools.MaxScopeFilesForTest + 40)}
	ts, _ := reviewToolsetOn(t, &stubThreadReader{}, stub)

	out, err := ts.Call(context.Background(), "get_review_scope", json.RawMessage(`{"limit":99999}`))
	require.NoError(t, err)

	require.Equal(t, agenttools.MaxScopeFilesForTest, fileRows(out))
}

// get_chat_log keeps the NEWEST turns, because what a sibling chat is doing now
// is why anyone reads it. The note has to say both how many older turns exist
// and that offset pages backwards — a model assuming forward offsets would page
// the wrong way and conclude the older history is not there.
func TestGetChatLog_KeepsTheMostRecentTurnsAndSaysWhatItDropped(t *testing.T) {
	total := 63
	logs := &stubChatLogs{turns: chatTurns(total)}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log", json.RawMessage(`{"chatId":"other"}`))
	require.NoError(t, err)

	kept := agenttools.DefaultChatLogTurnsForTest
	require.Contains(t, out, fmt.Sprintf(
		"Showing turns %d-%d of %d, oldest first. %d older turns not shown — call get_chat_log with offset=%d for the %d before these.",
		total-kept+1, total, total, total-kept, kept, kept))
	require.Contains(t, out, fmt.Sprintf("turn-%d", total))
	require.Contains(t, out, fmt.Sprintf("turn-%d", total-kept+1))
	require.NotContains(t, out, "turn-1\n", "the oldest turns are the ones the cap drops")
}

func TestGetChatLog_PagesBackwardsIntoOlderHistory(t *testing.T) {
	total := 63
	kept := agenttools.DefaultChatLogTurnsForTest
	logs := &stubChatLogs{turns: chatTurns(total)}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log",
		json.RawMessage(fmt.Sprintf(`{"chatId":"other","offset":%d}`, kept)))
	require.NoError(t, err)

	// The needles end at the newline that terminates a rendered turn: "turn-43"
	// on its own is a substring of "turn-430" and, worse, "turn-63" would match
	// nothing at all if spelled with a trailing colon, since a turn renders as
	// "user: turn-63" — an assertion that can never fire is not a guard.
	require.Contains(t, out, fmt.Sprintf("turn-%d\n", total-kept))
	require.Contains(t, out, fmt.Sprintf("turn-%d\n", total-(2*kept)+1))
	require.NotContains(t, out, fmt.Sprintf("turn-%d\n", total),
		"a backwards page must not re-render the turns the previous page already showed")
}

// Paging all the way back has to stop cleanly rather than pointing at an older
// page that does not exist.
func TestGetChatLog_ReachingTheStartSaysNothingIsOlder(t *testing.T) {
	total := 25
	logs := &stubChatLogs{turns: chatTurns(total)}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log",
		json.RawMessage(fmt.Sprintf(`{"chatId":"other","offset":%d}`, total-5)))
	require.NoError(t, err)

	require.Contains(t, out, fmt.Sprintf("Showing turns 1-5 of %d, oldest first. Nothing older exists.", total))
	require.Contains(t, out, "turn-1\n")
	require.NotContains(t, out, "turn-6\n", "the window still ends where the offset put it")
}

func TestGetChatLog_ClampsAnOversizedLimit(t *testing.T) {
	logs := &stubChatLogs{turns: chatTurns(agenttools.MaxChatLogTurnsForTest + 40)}
	ts := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-a1"}, logs)

	out, err := ts.Call(context.Background(), "get_chat_log",
		json.RawMessage(`{"chatId":"other","limit":99999}`))
	require.NoError(t, err)

	require.Equal(t, agenttools.MaxChatLogTurnsForTest,
		strings.Count(out, "user: turn-"))
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
	chatTS := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-a1"}, logs)
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
	chatTS := chatLogToolsOn(t, domain.AgentChat{ID: "other", WorkspaceID: "ws-b"}, logs)
	_, err = chatTS.Call(context.Background(), "get_chat_log",
		json.RawMessage(`{"chatId":"other","offset":0,"limit":100000}`))
	require.ErrorIs(t, err, agenttools.ErrOutOfScope)
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
