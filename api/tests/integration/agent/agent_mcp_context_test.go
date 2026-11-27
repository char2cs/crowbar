//go:build integration

package agent_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/tests/kit"
)

func (b *mcpBarrier) toolCalls(
	name string,
) []mcpCall {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]mcpCall, 0, len(b.calls))
	for _, c := range b.calls {
		if c.Method == "tools/call" && c.Tool == name {
			out = append(out, c)
		}
	}
	return out
}

const addressableBody = "This divides by `orders.length - 1`, which is wrong for every input and " +
	"divides by zero when there is exactly one order. Please divide by `orders.length` instead."

const pendingBody = "Separate question, not for this change: should this loop skip cancelled orders? " +
	"Finance has not told us yet — leaving this open until they answer."

const resolvePrompt = "There are review comments on this branch. Fix the one about the average being " +
	"divided by the wrong number, then mark that comment resolved."

func openThreadAt(
	t *testing.T,
	h *harness,
	wsID string,
	line int,
	body string,
) string {
	t.Helper()
	thread, err := h.app.Usecases.BranchReview.OpenThread(context.Background(), branchreview.OpenThreadInput{
		WsID:       wsID,
		FilePath:   reviewFile,
		LineNumber: line,
		Side:       domain.ReviewSideRight,
		Body:       body,
	})
	require.NoError(t, err, "open a review thread on %s:%d", reviewFile, line)
	return thread.ID
}

func TestMCP_ClaudeResolvesOnlyTheThreadItAddressed(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	wsID, defect := reviewFixture(t, h, "mcp-resolve")
	pendingLine := defect - 2
	requireAnchorable(t, h, wsID, reviewFile, pendingLine)

	addressed := openThreadAt(t, h, wsID, defect, addressableBody)
	pending := openThreadAt(t, h, wsID, pendingLine, pendingBody)
	require.NotEmpty(t, pending, "the control thread's id is the needle the discipline assertion looks for")
	t.Logf("threads: addressed=%s (line %d) pending=%s (line %d)", addressed, defect, pending, pendingLine)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	diagnoseOnFailure(t, h, tap, "claude")
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	priorTurns := assistantTurnCount(t, h, wsID, chatID, "claude")
	drive(t, h, tap, termSessID, resolvePrompt)
	awaitToolEffect(
		t, h, wsID, chatID, "claude", termSessID, tap, priorTurns,
		"claude to resolve review thread "+addressed+" through the crowbar tool surface",
		"while it was being asked to address and resolve a review comment",
		func() bool { return readThread(t, h, addressed).IsResolved() },
	)

	t.Logf("MCP traffic observed: %s", h.mcp.observed())

	resolves := h.mcp.toolCalls("resolve_review_thread")
	require.NotEmpty(t, resolves,
		"claude never called resolve_review_thread. It either resolved nothing, or reached the thread "+
			"by some other means — MCP traffic observed: %s", h.mcp.observed())
	require.True(t, readThread(t, h, addressed).IsResolved(),
		"the thread claude was asked to address is still open — MCP traffic observed: %s", h.mcp.observed())

	for _, call := range resolves {
		require.NotContains(t, call.Args, pending,
			"claude resolved the thread it was not asked about and had not addressed — the one that "+
				"says outright it is waiting on another team. The tool's description asks it to "+
				"resolve only what it has addressed; call was: %s", call)
	}
	require.False(t, readThread(t, h, pending).IsResolved(),
		"the pending thread must still be open")

	require.NotContains(t, reviewFileContent(t, h, wsID), buggyDivisor,
		"claude resolved the comment without fixing the code it was about")
}

func reviewFileContent(
	t *testing.T,
	h *harness,
	wsID string,
) string {
	t.Helper()
	ws, err := h.app.Repositories.Workspace.Get(context.Background(), wsID)
	require.NoError(t, err, "resolve workspace %s", wsID)
	data, err := os.ReadFile(filepath.Join(ws.WorktreePath, reviewFile))
	require.NoError(t, err, "read %s back out of the worktree", reviewFile)
	return string(data)
}

const (
	siblingChatTitle = "Payments Retry Investigation"
	siblingLogMarker = "RETRY-CEILING-7731"
)

var siblingLogTurns = []struct {
	role     string
	provider string
	text     string
}{
	{"user", "claude", "Why do payment retries stop after the second attempt?"},
	{
		"assistant", "claude",
		"I traced it to the retry ceiling in payments/retry.go: the loop exits once attempts == 2, " +
			"so the third attempt never runs. Tracking it as " + siblingLogMarker + ".",
	},
	{"user", "claude", "Is that ceiling configurable?"},
	{
		"assistant", "claude",
		"Not today — it is a literal. I am about to lift it into config and will report back.",
	},
}

func contextFixture(
	t *testing.T,
	h *harness,
	name string,
) (callerWsID, childWsID, chatID string) {
	t.Helper()
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	project, err := h.app.Usecases.ProjectImport.Create(ctx, "", name, repoPath)
	require.NoError(t, err, "create project")
	repo, err := h.app.Usecases.ProjectImport.ImportRepo(ctx, kit.RepoImport(project.ID, repoPath), kit.RequireBranches(t))
	require.NoError(t, err, "import repo")

	caller, err := h.app.Usecases.Workspace.CreateChild(ctx, workspace.CreateChildInput{
		RepoID:       repo.ID,
		ProjectID:    project.ID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       "agent-caller-branch",
		ParentBranch: repo.DefaultBranch,
	})
	require.NoError(t, err, "create the caller's workspace")

	child, err := h.app.Usecases.Workspace.CreateChild(ctx, workspace.CreateChildInput{
		RepoID:       repo.ID,
		ProjectID:    project.ID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       "agent-child-branch",
		ParentID:     caller.ID,
		ParentBranch: caller.Branch,
	})
	require.NoError(t, err, "create the child workspace")
	require.Equal(t, caller.ID, child.ParentID,
		"the child must hang off the caller, or it is not in the caller's visible set at all")

	chatID = seedSiblingChat(t, h, child)
	h.app.Repositories.WaitQuiescent()
	require.NotEmpty(t, chatID, "the sibling chat's id is the needle every traffic assertion looks for")
	return caller.ID, child.ID, chatID
}

func seedSiblingChat(
	t *testing.T,
	h *harness,
	ws domain.Workspace,
) string {
	t.Helper()
	ctx := context.Background()

	chatID := uuid.NewString()
	_, err := h.app.Repositories.AgentChat.Create(ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: ws.ID, Type: domain.ChatTypeChat, Now: time.Now().UTC(),
	})
	require.NoError(t, err, "create the sibling chat")
	require.NoError(t,
		h.app.Usecases.AgentChat.RenameChat(ctx, chatID, siblingChatTitle, "user"),
		"title the sibling chat")

	turnID := ""
	for i, turn := range siblingLogTurns {
		var rows []ledger.Row
		if turn.role == domain.TurnRoleUser {
			turnID = uuid.NewString()
			rows = []ledger.Row{
				ledgerRow(t, ledger.KindUserMessage, turnID, ledger.UserMessage{Text: turn.text, Origin: ledger.OriginProvider}),
				ledgerRow(t, ledger.KindTurnStarted, turnID, ledger.TurnStarted{Provider: turn.provider}),
			}
		} else {
			rows = []ledger.Row{
				ledgerRow(t, ledger.KindAssistantText, turnID, ledger.AssistantText{
					MessageID: ledger.ReplyID(turnID), Index: ledger.IndexComplete, Text: turn.text,
				}),
				ledgerRow(t, ledger.KindTurnEnded, turnID, ledger.TurnEnded{Status: string(ledger.TurnStatusDone)}),
			}
		}
		_, _, err := h.app.Repositories.ChatLedger.Append(ctx, chatID, rows...)
		require.NoError(t, err, "append the sibling chat's turn %d", i)
	}
	h.app.Repositories.WaitQuiescent()

	turns, err := h.app.Usecases.AgentChat.ReadChatLog(ctx, chatID)
	require.NoError(t, err, "read the seeded log back through the tool's own reader")
	require.Len(t, turns, len(siblingLogTurns),
		"the seeded record must be readable through the path get_chat_log serves from")
	require.Contains(t, renderChatLog(turns), siblingLogMarker,
		"the marker the traffic assertion hunts for must be in what get_chat_log's own reader serves")
	return chatID
}

func ledgerRow(t *testing.T, kind ledger.Kind, turnID string, body any) ledger.Row {
	t.Helper()
	row, err := ledger.NewRow(kind, turnID, "", body)
	require.NoError(t, err)
	return row
}

func renderChatLog(
	turns []agentusecase.ChatTurn,
) string {
	out := make([]string, 0, len(turns))
	for _, turn := range turns {
		out = append(out, turn.Speaker+": "+turn.Body)
	}
	return strings.Join(out, "\n")
}

const contextPrompt = "Another agent has been working in a workspace under this one. " +
	"Find its chat, read its log, and tell me in one sentence what it found."

func TestMCP_ClaudeReadsASiblingChatLogAcrossWorkspaces(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	callerWsID, childWsID, siblingChatID := contextFixture(t, h, "mcp-context")
	t.Logf("hierarchy: caller=%s child=%s sibling chat=%s", callerWsID, childWsID, siblingChatID)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, callerWsID, "claude")
	diagnoseOnFailure(t, h, tap, "claude")
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, callerWsID, h.home)

	priorTurns := assistantTurnCount(t, h, callerWsID, chatID, "claude")
	drive(t, h, tap, termSessID, contextPrompt)
	awaitToolEffect(
		t, h, callerWsID, chatID, "claude", termSessID, tap, priorTurns,
		"claude to read the sibling chat's log through the crowbar tool surface",
		"while it was being asked to find another agent's chat",
		func() bool { return h.mcp.calledTool("get_chat_log") },
	)

	t.Logf("MCP traffic observed: %s", h.mcp.observed())

	require.True(t, h.mcp.calledTool("list_workspaces"),
		"claude never called list_workspaces, so it cannot have discovered the sibling chat's id from "+
			"this surface — MCP traffic observed: %s", h.mcp.observed())

	reads := h.mcp.toolCalls("get_chat_log")
	require.NotEmpty(t, reads,
		"claude never called get_chat_log — MCP traffic observed: %s", h.mcp.observed())
	require.True(t, anyCallReadChat(reads, siblingChatID),
		"get_chat_log was called, but never for the sibling chat %s on the child workspace — the calls "+
			"were: %s", siblingChatID, callSummary(reads))
	require.True(t, anyReplyContains(reads, siblingLogMarker),
		"no get_chat_log reply carried the marker the sibling's record holds, so the log the daemon "+
			"served was not that chat's — the calls were: %s", callSummary(reads))

	t.Logf("claude's own answer: %s", lastAssistantReply(t, h, callerWsID, chatID, "claude"))
}

func anyCallReadChat(
	calls []mcpCall,
	chatID string,
) bool {
	for _, c := range calls {
		if strings.Contains(c.Args, chatID) {
			return true
		}
	}
	return false
}

func anyReplyContains(
	calls []mcpCall,
	needle string,
) bool {
	for _, c := range calls {
		if strings.Contains(c.Reply, needle) {
			return true
		}
	}
	return false
}

func callSummary(
	calls []mcpCall,
) string {
	out := make([]string, 0, len(calls))
	for _, c := range calls {
		out = append(out, c.String())
	}
	return strings.Join(out, ", ")
}

func lastAssistantReply(
	t *testing.T,
	h *harness,
	wsID string,
	chatID string,
	provider string,
) string {
	t.Helper()
	replies := assistantReplies(readLedgerTurns(t, h, wsID, chatID), provider)
	if len(replies) == 0 {
		return "(the turn had not finished when the tool call satisfied the wait)"
	}
	return fmt.Sprintf("%q", replies[len(replies)-1])
}
