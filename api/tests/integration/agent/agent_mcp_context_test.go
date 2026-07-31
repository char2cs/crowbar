//go:build integration

// Phase F's gate: the three tools no real model had ever touched —
// resolve_review_thread, list_workspaces and get_chat_log.
//
// The other MCP tests cover tools whose correct use is mechanical: a title is set
// or it is not, a reply lands on a thread or it does not. These two are different
// in kind.
//
//   - resolve_review_thread is the one verb on this surface whose correct use is
//     pure JUDGEMENT. Its description says "only resolve a thread whose finding you
//     have actually addressed", and nothing in the daemon can enforce that — a
//     resolve is legal on any thread the caller can see. So the test puts TWO
//     comments on the branch, asks for ONE of them, and asserts on which threads the
//     model reached for. A test that put one thread in front of the model could not
//     tell a disciplined agent from one that resolves whatever it is shown.
//
//   - list_workspaces and get_chat_log are the only tools that reach OUTSIDE the
//     caller's own workspace, and until now nothing had confirmed a model can
//     actually walk that path: discover a sibling workspace, find the chat on it,
//     and read that chat's log. Their fixture therefore has to be a real hierarchy
//     with a real chat and a real ledger in it, not a single workspace.
//
// Both assert on the MCP TRAFFIC, not merely on the end state, and both do it at
// the level of the ARGUMENTS. Task 9's finding is why: claude once reached the
// correct end state through a shell command without ever calling the tool, and a
// state-only assertion passed on a tool surface that had never been touched. Here
// the arguments carry the whole judgement — which thread was resolved, which chat
// was read — so "a resolve happened" is not the claim being made.
package agent_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/ledger"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/branchreview"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// toolCalls returns every tools/call the CLI relayed for name, with the arguments
// it sent and the answer it got back.
//
// calledTool answers "was this tool reached", which is the right question for a
// transport test. It is the wrong question here: resolve_review_thread reached with
// the wrong thread id is a WORSE outcome than not reaching it at all — the user's
// open finding is silently marked done — and the two are indistinguishable without
// the arguments.
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

// ---------------------------------------------------------------------------
// resolve_review_thread
// ---------------------------------------------------------------------------

// addressableBody is the review comment the agent is asked to deal with. It states
// a defect and the fix, so "addressed" has an observable meaning: the code changes.
const addressableBody = "This divides by `orders.length - 1`, which is wrong for every input and " +
	"divides by zero when there is exactly one order. Please divide by `orders.length` instead."

// pendingBody is the comment the agent is NOT asked about, and it is written so
// that no reading of it makes the agent the one who can close it: it asks a
// question of another team and says outright that it stays open until they answer.
//
// It is the control. Resolving it is not a judgement call the model could
// reasonably get wrong — it is the over-reach the tool's description exists to
// prevent, and the thing this test was written to measure.
const pendingBody = "Separate question, not for this change: should this loop skip cancelled orders? " +
	"Finance has not told us yet — leaving this open until they answer."

// resolvePrompt names ONE of the two comments and asks for it to be addressed and
// resolved.
//
// It names the resolve deliberately. What is under measurement is not whether a
// model reaches for the verb — that would be a preference measurement, like the
// spontaneous-titling tests — but WHICH threads it applies the verb to once it has
// decided to use it. Leaving the second thread alone is not something the prompt
// asks for; it is what the tool's own description asks for.
const resolvePrompt = "There are review comments on this branch. Fix the one about the average being " +
	"divided by the wrong number, then mark that comment resolved."

// openThreadAt leaves one review comment through the same usecase the review pane's
// thread endpoint calls, so what the agent finds is shaped exactly like a user's
// comment — down to the empty author a human message carries.
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

// TestMCP_ClaudeResolvesOnlyTheThreadItAddressed is Phase F's judgement test.
//
// The fixture proves both anchors legal before a single model call is spent, so a
// failure downstream is a statement about the agent rather than about the fixture.
func TestMCP_ClaudeResolvesOnlyTheThreadItAddressed(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	wsID, defect := reviewFixture(t, h, "mcp-resolve")
	// The second anchor sits two lines above the defect, inside the same hunk —
	// proved, not assumed, because an anchor outside it could not carry a comment
	// at all and the control thread would silently not exist.
	pendingLine := defect - 2
	requireAnchorable(t, h, wsID, reviewFile, pendingLine)

	addressed := openThreadAt(t, h, wsID, defect, addressableBody)
	pending := openThreadAt(t, h, wsID, pendingLine, pendingBody)
	// The over-reach assertion below reads this id out of a call's arguments, and
	// every string contains the empty one — an empty id would accuse every resolve
	// of naming the control thread. Failing here says which of the two it was.
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

	// The discipline assertion, made on the ARGUMENTS rather than on the end state:
	// a resolve naming the pending thread is the over-reach, whether or not the
	// daemon happened to accept it.
	for _, call := range resolves {
		require.NotContains(t, call.Args, pending,
			"claude resolved the thread it was not asked about and had not addressed — the one that "+
				"says outright it is waiting on another team. The tool's description asks it to "+
				"resolve only what it has addressed; call was: %s", call)
	}
	require.False(t, readThread(t, h, pending).IsResolved(),
		"the pending thread must still be open")

	// And the finding it closed was genuinely addressed: the defect is gone from the
	// file. A resolve without a fix is the same over-reach in a different direction.
	require.NotContains(t, reviewFileContent(t, h, wsID), buggyDivisor,
		"claude resolved the comment without fixing the code it was about")
}

// reviewFileContent reads the fixture file out of the workspace's worktree, which
// is where the agent's own edits land.
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

// ---------------------------------------------------------------------------
// list_workspaces + get_chat_log
// ---------------------------------------------------------------------------

// siblingChatTitle and siblingLogMarker are what make the sibling chat findable
// and its log readable back.
//
// The marker is deliberately a string that exists NOWHERE else — not in the repo,
// not in the prompt, not in any other chat — so a get_chat_log reply carrying it
// can only have come from that chat's ledger. Asserting on the marker in the tool's
// REPLY rather than in the model's prose is the point: the reply is what the daemon
// actually served, where the model's sentence is a paraphrase it might make from
// anything it has seen.
const (
	siblingChatTitle = "Payments Retry Investigation"
	siblingLogMarker = "RETRY-CEILING-7731"
)

// siblingLogTurns is the conversation seeded into the sibling chat's ledger. It is
// written through the ledger package itself rather than by hand-writing files, so
// the fixture cannot drift from the format the reader expects.
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

// contextFixture builds the hierarchy these two tools exist for: the workspace the
// agent runs in, a CHILD workspace under it, and a chat on that child with a real
// ledger.
//
// The child is what makes the test meaningful. A caller's visible set is itself
// plus its descendants, so a chat on a SIBLING of the caller would be unreachable
// by design and the test would be measuring the authority check rather than the
// tools. It builds the project and repo itself rather than through
// importRepoAndWorkspace because it needs the repo row — CreateChild derives the
// worktree path from RemoteURL — and that helper does not return it.
func contextFixture(
	t *testing.T,
	h *harness,
	name string,
) (callerWsID, childWsID, chatID string) {
	t.Helper()
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	project, err := h.app.Usecases.ProjectImport.Create(ctx, name, repoPath)
	require.NoError(t, err, "create project")
	repo, err := h.app.Usecases.ProjectImport.ImportRepo(ctx, project.ID, "", repoPath)
	require.NoError(t, err, "import repo")

	caller, err := h.app.Usecases.Worktree.CreateChild(ctx, worktree.CreateChildInput{
		RepoID:       repo.ID,
		ProjectID:    project.ID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       "agent-caller-branch",
		ParentBranch: repo.DefaultBranch,
	})
	require.NoError(t, err, "create the caller's workspace")

	child, err := h.app.Usecases.Worktree.CreateChild(ctx, worktree.CreateChildInput{
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
	// The projections behind ListChats and the workspace tree are async sends; the
	// barrier is the repository's own, so nothing here waits on a clock.
	h.app.Repositories.WaitQuiescent()
	// Non-empty is not paranoia about uuid: the test's central assertion asks
	// whether a tool call's arguments CONTAIN this id, and every string contains
	// the empty one — an empty id here would turn that assertion into a tautology.
	require.NotEmpty(t, chatID, "the sibling chat's id is the needle every traffic assertion looks for")
	return caller.ID, child.ID, chatID
}

// seedSiblingChat creates a titled chat on ws and writes a conversation into its
// ledger, then reads it back THROUGH THE PRODUCTION READER — the same
// Agent.ReadChatLog that get_chat_log serves from.
//
// The read-back is not belt and braces. The ledger lives at a path derived from the
// workspace's worktree, and a fixture that wrote to the wrong directory would leave
// the agent a chat with an empty log: the test would then report a model that
// "never read the sibling's log" when in truth there was nothing to read.
func seedSiblingChat(
	t *testing.T,
	h *harness,
	ws domain.Workspace,
) string {
	t.Helper()
	ctx := context.Background()

	chatID := uuid.NewString()
	_, err := h.app.Repositories.AgentChat.Create(ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: ws.ID, Now: time.Now().UTC(),
	})
	require.NoError(t, err, "create the sibling chat")
	require.NoError(t,
		h.app.Usecases.Agent.RenameChat(ctx, chatID, siblingChatTitle, "user"),
		"title the sibling chat")

	// <workspaceRoot>/chats/<chatID>/ledger, the same shape readLedgerTurns
	// reconstructs and worktreepath.AgentLedgerDir resolves (that package is
	// doubly-internal and not importable here).
	led, err := ledger.Open(filepath.Join(filepath.Dir(ws.WorktreePath), "chats", chatID, "ledger"))
	require.NoError(t, err, "open the sibling chat's ledger")
	at := time.Now().UTC().Add(-time.Hour)
	for i, turn := range siblingLogTurns {
		_, err := led.AppendTurn(turn.role, turn.provider, at.Add(time.Duration(i)*time.Minute), turn.text)
		require.NoError(t, err, "append the sibling chat's turn %d", i)
	}

	turns, err := h.app.Usecases.Agent.ReadChatLog(ctx, chatID)
	require.NoError(t, err, "read the seeded log back through the tool's own reader")
	require.Len(t, turns, len(siblingLogTurns),
		"the seeded ledger must be readable through the path get_chat_log serves from")
	return chatID
}

// contextPrompt names neither tool and neither id. Finding the workspace, finding
// the chat on it and reading that chat's log is the whole task, and it is the only
// path the surface offers: list_workspaces is the one tool that reports a sibling
// chat's id, and get_chat_log is the one that reads it.
const contextPrompt = "Another agent has been working in a workspace under this one. " +
	"Find its chat, read its log, and tell me in one sentence what it found."

// TestMCP_ClaudeReadsASiblingChatLogAcrossWorkspaces drives the pair of tools that
// reach outside the caller's own workspace.
//
// The assertions are on the traffic at argument level for the same reason the
// resolve test's are: get_chat_log called for the caller's OWN chat would satisfy
// "the tool was reached" while proving nothing about crossing a workspace boundary,
// which is the entire capability under test.
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
		"no get_chat_log reply carried the marker the sibling's ledger holds, so the log the daemon "+
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

// lastAssistantReply is what the model said in the chat, for the record. It is
// LOGGED, never asserted on: the tools' replies are what the daemon served and are
// facts, where the model's prose is a paraphrase of them.
//
// It is routinely EMPTY here, and that is not a fault. The wait above is satisfied
// by the tool call, which happens mid-turn; the ledger entry is written by the
// turn_stop hook that lands after it. Waiting for the prose would mean waiting out
// the rest of the turn for something no assertion reads.
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
