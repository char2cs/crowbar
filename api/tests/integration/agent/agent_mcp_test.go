//go:build integration

package agent_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/tests/kit"
)

const wantTitle = "Widget Refactor"

const titlePrompt = "Call your set_chat_title tool with the title: Widget Refactor. " +
	"Use the tool itself and run no shell command."

type mcpCall struct {
	Method  string
	Tool    string
	Args    string
	Reply   string
	IsError bool
}

func (c mcpCall) String() string {
	if c.Tool == "" {
		return c.Method
	}
	out := c.Method + "(" + c.Tool + " " + c.Args + ")"
	if c.IsError {
		return out + " -> REFUSED: " + c.Reply
	}
	if c.Reply == "" {
		return out
	}
	return out + " -> " + c.Reply
}

type mcpToolReply struct {
	Data struct {
		RPC struct {
			Result struct {
				Content []mcpTextContent `json:"content"`
				IsError bool             `json:"isError"`
			} `json:"result"`
		} `json:"rpc"`
	} `json:"data"`
}

type mcpTextContent struct {
	Text string `json:"text"`
}

func (r mcpToolReply) text() string {
	parts := make([]string, 0, len(r.Data.RPC.Result.Content))
	for _, c := range r.Data.RPC.Result.Content {
		parts = append(parts, c.Text)
	}
	return strings.Join(parts, " ")
}

type responseTap struct {
	gin.ResponseWriter
	body *bytes.Buffer
}

func (w *responseTap) Write(
	p []byte,
) (int, error) {
	w.body.Write(p)
	return w.ResponseWriter.Write(p)
}

func (w *responseTap) WriteString(
	s string,
) (int, error) {
	w.body.WriteString(s)
	return w.ResponseWriter.WriteString(s)
}

type mcpBarrier struct {
	sig   *kit.Signal
	mu    sync.Mutex
	calls []mcpCall
}

func newMCPBarrier() *mcpBarrier {
	return &mcpBarrier{sig: kit.NewSignal()}
}

func (b *mcpBarrier) middleware() gin.HandlerFunc {
	return func(ctx *gin.Context) { b.observe(ctx) }
}

func (b *mcpBarrier) observe(
	ctx *gin.Context,
) {
	if ctx.Request.Method != http.MethodPost || !strings.HasSuffix(ctx.FullPath(), "/mcp") {
		ctx.Next()
		return
	}
	at := b.record(snapshotBody(ctx))
	tap := &responseTap{ResponseWriter: ctx.Writer, body: &bytes.Buffer{}}
	ctx.Writer = tap
	ctx.Next()
	b.recordReply(at, tap.body.Bytes())
	b.sig.Fire()
}

func (b *mcpBarrier) record(
	raw []byte,
) int {
	var body struct {
		RPC struct {
			Method string `json:"method"`
			Params struct {
				Name      string          `json:"name"`
				Arguments json.RawMessage `json:"arguments"`
			} `json:"params"`
		} `json:"rpc"`
	}
	_ = json.Unmarshal(raw, &body)

	b.mu.Lock()
	defer b.mu.Unlock()
	b.calls = append(b.calls, mcpCall{
		Method: body.RPC.Method,
		Tool:   body.RPC.Params.Name,
		Args:   string(body.RPC.Params.Arguments),
	})
	return len(b.calls) - 1
}

func (b *mcpBarrier) recordReply(
	at int,
	raw []byte,
) {
	var reply mcpToolReply
	if json.Unmarshal(raw, &reply) != nil {
		return
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if at < 0 || at >= len(b.calls) || b.calls[at].Method != "tools/call" {
		return
	}
	b.calls[at].Reply = reply.text()
	b.calls[at].IsError = reply.Data.RPC.Result.IsError
}

func (b *mcpBarrier) calledTool(
	name string,
) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, c := range b.calls {
		if c.Method == "tools/call" && c.Tool == name {
			return true
		}
	}
	return false
}

func (b *mcpBarrier) observed() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.calls) == 0 {
		return "(none — the CLI never relayed a single MCP message)"
	}
	out := make([]string, 0, len(b.calls))
	for _, c := range b.calls {
		out = append(out, c.String())
	}
	return strings.Join(out, ", ")
}

func snapshotBody(
	ctx *gin.Context,
) []byte {
	raw, err := io.ReadAll(ctx.Request.Body)
	if err != nil {
		return nil
	}
	ctx.Request.Body = io.NopCloser(bytes.NewReader(raw))
	return raw
}

func awaitToolTitle(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
	want string,
	priorTurns int,
	termSessID string,
	tap *kit.PTYTap,
) {
	t.Helper()
	awaitToolEffect(
		t, h, wsID, chatID, provider, termSessID, tap, priorTurns,
		provider+" to title its chat "+want+" through the crowbar tool surface",
		"while it was being asked to title its chat",
		func() bool { return chatTitle(t, h, chatID) == want },
	)
}

func awaitToolEffect(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
	termSessID string,
	tap *kit.PTYTap,
	priorTurns int,
	what string,
	when string,
	effect func() bool,
) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), backstop)
	defer cancel()

	kit.Await(t, ctx, what, func() (bool, bool) {
		h.app.Repositories.WaitQuiescent()
		if effect() {
			return true, true
		}
		if assistantTurnCount(t, h, wsID, chatID, provider) > priorTurns && !chatWorking(t, h, chatID) {
			return false, true
		}
		requireCLIAlive(t, h, tap, termSessID, provider, when)
		return false, false
	}, h.hooks.sig, h.mcp.sig)
}

func assistantTurnCount(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
) int {
	t.Helper()
	return len(assistantReplies(readLedgerTurns(t, h, wsID, chatID), provider))
}

func chatWorking(
	t *testing.T,
	h *harness,
	chatID string,
) bool {
	t.Helper()
	snap, err := h.app.Usecases.AgentChat.ChatSnapshot(context.Background(), chatID)
	require.NoError(t, err, "read chat %s back", chatID)
	return snap.Ledger.Busy
}

func chatTitle(
	t *testing.T,
	h *harness,
	chatID string,
) string {
	t.Helper()
	chat, err := h.app.Usecases.AgentChat.GetChat(context.Background(), chatID)
	require.NoError(t, err, "read chat %s back", chatID)
	return chat.Title
}

func diagnoseOnFailure(
	t *testing.T,
	h *harness,
	tap *kit.PTYTap,
	provider string,
) {
	t.Helper()
	t.Cleanup(func() {
		if !t.Failed() {
			return
		}
		t.Logf("MCP traffic observed: %s", h.mcp.observed())
		t.Logf("final %s screen:\n%s", provider, kit.Readable(tap.Screen()))
	})
}

func TestMCP_ClaudeTitlesItsChatThroughTheToolSurface(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "mcp-claude", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	diagnoseOnFailure(t, h, tap, "claude")
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	priorTurns := assistantTurnCount(t, h, wsID, chatID, "claude")
	drive(t, h, tap, termSessID, titlePrompt)
	awaitToolTitle(t, h, wsID, chatID, "claude", wantTitle, priorTurns, termSessID, tap)

	require.True(t, h.mcp.calledTool("set_chat_title"),
		"claude never called set_chat_title through the crowbar MCP surface. Either the inline --mcp-config "+
			"was not honoured, the relay could not reach the daemon, or the model declined the tool")
	require.Equal(t, wantTitle, chatTitle(t, h, chatID),
		"the chat's title must be the one claude set through set_chat_title")
}

func TestMCP_CodexTitlesItsChatThroughTheToolSurface(t *testing.T) {
	requireCLI(t, "codex")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "mcp-codex", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "codex")
	diagnoseOnFailure(t, h, tap, "codex")
	t.Logf("spawned codex: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	priorTurns := assistantTurnCount(t, h, wsID, chatID, "codex")
	drive(t, h, tap, termSessID, titlePrompt)
	awaitToolTitle(t, h, wsID, chatID, "codex", wantTitle, priorTurns, termSessID, tap)

	require.True(t, h.mcp.calledTool("set_chat_title"),
		"codex never called set_chat_title through the crowbar MCP surface. Either the -c mcp_servers.crowbar.* "+
			"overrides did not register the server, the relay could not reach the daemon, or the model declined "+
			"the tool")
	require.Equal(t, wantTitle, chatTitle(t, h, chatID),
		"the chat's title must be the one codex set through set_chat_title")
}

func TestMCP_ForgedTokenIsRejected(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)
	ctx := context.Background()

	_, _, _, chatID, runnerID := mustSpawnChat(t, h, "claude")
	before := chatTitle(t, h, chatID)

	out, send, err := h.app.Usecases.AgentProvider.DispatchMCP(ctx, runnerID, "forged",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":`+
			`{"name":"set_chat_title","arguments":{"title":"Nope"}}}`))
	require.NoError(t, err)
	require.True(t, send, "a request must be answered, never met with silence")
	require.Contains(t, string(out), "unauthorized",
		"a forged token must be refused by the resolver before any tool runs")

	require.NotContains(t, string(out), "Chat titled",
		"the tool must not have run at all")

	h.app.Repositories.WaitQuiescent()
	require.Equal(t, before, chatTitle(t, h, chatID),
		"a refused call must write nothing: the chat's title must be exactly what it was before")
}

const reviewFile = "billing.js"

const reviewFileBase = `// Order helpers used by the billing report.

function orderTotal(order) {
  let total = 0;
  for (const line of order.lines) {
    total += line.quantity * line.unitPrice;
  }
  return total;
}

function formatAmount(cents) {
  return "$" + (cents / 100).toFixed(2);
}

function reportLines(orders) {
  const out = [];
  for (const order of orders) {
    out.push({
      id: order.id,
      total: formatAmount(orderTotal(order)),
    });
  }
  return out;
}

function averageOrderValue(orders) {
  if (orders.length === 0) {
    return 0;
  }
  let total = 0;
  for (const order of orders) {
    total += orderTotal(order);
  }
  return total / orders.length;
}

module.exports = { orderTotal, formatAmount, reportLines, averageOrderValue };
`

const (
	correctDivisor = "  return total / orders.length;"
	buggyDivisor   = "  return total / (orders.length - 1);"
)

const userThreadBody = "Why does this divide by `orders.length - 1`? With exactly one order " +
	"that divides by zero. Was it deliberate?"

const (
	reviewReplyPrompt   = "There is a review comment on this branch. Read it and reply to it."
	reviewFindingPrompt = "Review this branch and post any finding as a review comment."
)

func reviewFixture(
	t *testing.T,
	h *harness,
	name string,
) (string, int) {
	t.Helper()

	repoPath := kit.InitRepoWithFile(t, reviewFile, reviewFileBase)
	_, _, wsID := h.importRepoAndWorkspace(t, name, repoPath)

	ws, err := h.app.Repositories.Workspace.Get(context.Background(), wsID)
	require.NoError(t, err, "resolve the fixture workspace's worktree")
	branchContent := buggyVariant(t)
	kit.CommitFile(t, ws.WorktreePath, reviewFile, branchContent, "average orders over the wrong divisor")

	require.Contains(t, changedFiles(t, h, wsID), reviewFile,
		"the fixture's committed change must appear in the branch review, or the agent has nothing to review")
	line := defectLine(t, branchContent)
	requireAnchorable(t, h, wsID, reviewFile, line)
	return wsID, line
}

func buggyVariant(
	t *testing.T,
) string {
	t.Helper()
	out := strings.Replace(reviewFileBase, correctDivisor, buggyDivisor, 1)
	require.NotEqual(t, reviewFileBase, out,
		"the fixture's base file no longer contains %q, so the branch would carry no change at all", correctDivisor)
	return out
}

func defectLine(
	t *testing.T,
	content string,
) int {
	t.Helper()
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, buggyDivisor) {
			return i + 1
		}
	}
	t.Fatalf("the fixture's branch content does not contain %q", buggyDivisor)
	return 0
}

func requireAnchorable(
	t *testing.T,
	h *harness,
	wsID string,
	path string,
	line int,
) {
	t.Helper()
	outline, err := h.app.Usecases.BranchReview.GetOutline(context.Background(), wsID, "")
	require.NoError(t, err, "read the review's hunk geometry for workspace %s", wsID)
	for _, f := range outline {
		if f.Path == path && hunkCovers(f.Hunks, line) {
			t.Logf("fixture geometry: %s hunks=%+v, defect on right-side line %d", path, f.Hunks, line)
			return
		}
	}
	t.Fatalf("line %d of %s is in no changed hunk of this review, so no anchored comment could ever be "+
		"posted there; outline observed: %+v", line, path, outline)
}

func hunkCovers(
	hunks []gitdomain.HunkShape,
	line int,
) bool {
	for _, h := range hunks {
		if line >= h.NewStart && line <= h.NewStart+h.NewLines-1 {
			return true
		}
	}
	return false
}

func openUserThread(
	t *testing.T,
	h *harness,
	wsID string,
	line int,
) string {
	t.Helper()
	thread, err := h.app.Usecases.BranchReview.OpenThread(context.Background(), branchreview.OpenThreadInput{
		WsID:       wsID,
		FilePath:   reviewFile,
		LineNumber: line,
		Side:       domain.ReviewSideRight,
		Body:       userThreadBody,
	})
	require.NoError(t, err, "open the user's review thread")
	return thread.ID
}

func readThread(
	t *testing.T,
	h *harness,
	threadID string,
) domain.ReviewThread {
	t.Helper()
	thread, err := h.app.Repositories.ReviewThread.Get(context.Background(), threadID)
	require.NoError(t, err, "read review thread %s back", threadID)
	return thread
}

func agentThreads(
	t *testing.T,
	h *harness,
	wsID string,
) []domain.ReviewThread {
	t.Helper()
	threads, err := h.app.Repositories.ReviewThread.ListByWorkspace(context.Background(), wsID)
	require.NoError(t, err, "list the review threads on workspace %s", wsID)
	out := make([]domain.ReviewThread, 0, len(threads))
	for _, thread := range threads {
		if len(thread.Messages) > 0 && thread.Messages[0].IsAgent {
			out = append(out, thread)
		}
	}
	return out
}

func changedFiles(
	t *testing.T,
	h *harness,
	wsID string,
) []string {
	t.Helper()
	files, err := h.app.Usecases.BranchReview.GetFiles(context.Background(), wsID, "")
	require.NoError(t, err, "read the review's changed files for workspace %s", wsID)
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Path)
	}
	return out
}

func TestMCP_ClaudeAnswersAReviewThread(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	wsID, line := reviewFixture(t, h, "mcp-review-reply")
	threadID := openUserThread(t, h, wsID, line)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	diagnoseOnFailure(t, h, tap, "claude")
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s thread=%s home=%s",
		chatID, runnerID, wsID, threadID, h.home)

	priorTurns := assistantTurnCount(t, h, wsID, chatID, "claude")
	drive(t, h, tap, termSessID, reviewReplyPrompt)
	awaitToolEffect(
		t, h, wsID, chatID, "claude", termSessID, tap, priorTurns,
		"claude to answer review thread "+threadID+" through the crowbar tool surface",
		"while it was being asked to answer a review comment",
		func() bool { return len(readThread(t, h, threadID).Messages) > 1 },
	)

	require.True(t, h.mcp.calledTool("reply_to_review_thread"),
		"claude never called reply_to_review_thread. It either answered in chat prose instead of in the review, "+
			"or reached the thread by some other means — MCP traffic observed: %s", h.mcp.observed())
	thread := readThread(t, h, threadID)
	require.Len(t, thread.Messages, 2,
		"the user's thread must now carry exactly two messages: the comment and the agent's answer")
	require.True(t, thread.Messages[1].IsAgent,
		"the answer must be recorded as the AGENT's, or the review UI attributes it to the user")

	t.Logf("MCP traffic observed: %s", h.mcp.observed())
	t.Logf("agent's answer: %s", thread.Messages[1].Body)
}

func TestMCP_ClaudePostsAFindingAsAnAnchoredThread(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	wsID, _ := reviewFixture(t, h, "mcp-review-post")

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	diagnoseOnFailure(t, h, tap, "claude")
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	priorTurns := assistantTurnCount(t, h, wsID, chatID, "claude")
	drive(t, h, tap, termSessID, reviewFindingPrompt)
	awaitToolEffect(
		t, h, wsID, chatID, "claude", termSessID, tap, priorTurns,
		"claude to post a finding as an anchored review thread",
		"while it was being asked to review the branch",
		func() bool { return len(agentThreads(t, h, wsID)) > 0 },
	)

	require.True(t, h.mcp.calledTool("post_review_comment"),
		"claude never called post_review_comment. It either wrote its findings as chat prose, or reviewed "+
			"nothing at all — MCP traffic observed: %s", h.mcp.observed())
	threads := agentThreads(t, h, wsID)
	require.NotEmpty(t, threads,
		"post_review_comment was called but no agent thread exists on the workspace, so every call was "+
			"REFUSED — MCP traffic observed: %s", h.mcp.observed())

	changed := changedFiles(t, h, wsID)
	for _, thread := range threads {
		require.Contains(t, changed, thread.FilePath,
			"the agent anchored a finding to %s, which is not in this review's changed files %v",
			thread.FilePath, changed)
		t.Logf("agent finding: %s:%d-%d (%s side): %s",
			thread.FilePath, thread.StartLine, thread.EndLine, thread.Side, thread.Messages[0].Body)
	}

	t.Logf("MCP traffic observed: %s", h.mcp.observed())
}

const taskPrompt = "Read README.md and tell me in one sentence what this repository is."

func spontaneousTitling(
	t *testing.T,
	provider string,
) {
	t.Helper()
	requireCLI(t, provider)
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "mcp-spontaneous-"+provider, repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, provider)
	diagnoseOnFailure(t, h, tap, provider)
	t.Logf("spawned %s: chat=%s runner=%s workspace=%s", provider, chatID, runnerID, wsID)

	derived := chatTitle(t, h, chatID)
	priorTurns := assistantTurnCount(t, h, wsID, chatID, provider)
	drive(t, h, tap, termSessID, taskPrompt)

	awaitToolEffect(t, h, wsID, chatID, provider, termSessID, tap, priorTurns,
		provider+" to title its own chat unprompted",
		"while it worked on a task that never mentioned titling",
		func() bool { return h.mcp.calledTool("set_chat_title") })

	require.True(t, h.mcp.calledTool("set_chat_title"),
		"%s finished a turn without titling its chat. The capability preamble asks for a title and "+
			"set_chat_title is registered, so either the ask is not reaching the model or it declined "+
			"it — MCP traffic observed: %s", provider, h.mcp.observed())

	got := chatTitle(t, h, chatID)
	require.NotEqual(t, derived, got,
		"set_chat_title was called but the title is unchanged from the one the daemon derived at "+
			"prompt submission, so the call did not take effect")
	t.Logf("%s titled its chat unprompted: %q (was %q). MCP traffic: %s",
		provider, got, derived, h.mcp.observed())
}

func TestMCP_ClaudeTitlesItsChatWithoutBeingAsked(t *testing.T) {
	spontaneousTitling(t, "claude")
}

func TestMCP_CodexTitlesItsChatWithoutBeingAsked(t *testing.T) {
	spontaneousTitling(t, "codex")
}
