package chat_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/core/config"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// ─── from rename_test.go ──────────────────────────────────────────────

// getTitle returns chatID's persisted Title for assertions. It waits for
// quiescence first so the read observes the latest SetTitle (and so a
// subsequent RenameChat's own read is against caught-up state).
func getTitle(t *testing.T, f testFixture, chatID string) string {
	t.Helper()
	c := f.chat(t, chatID)
	return c.Title
}

// TestRenameChat_Precedence guards RenameChat's user>agent>derived precedence:
// derived only fills an empty title, agent may upgrade a derived title but
// never a user-locked one, and a user rename always wins and locks the title.
func TestRenameChat_Precedence(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	// derived sets when empty, then does not overwrite
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "First Topic", "derived"))
	assert.Equal(t, "First Topic", getTitle(t, f, chatID))
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Second Topic", "derived"))
	assert.Equal(t, "First Topic", getTitle(t, f, chatID))

	// agent upgrades a derived title
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Agent Title", "agent"))
	assert.Equal(t, "Agent Title", getTitle(t, f, chatID))

	// user rename wins and locks; agent can no longer clobber
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "User Title", "user"))
	assert.Equal(t, "User Title", getTitle(t, f, chatID))
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Agent Again", "agent"))
	assert.Equal(t, "User Title", getTitle(t, f, chatID))

	// empty is always a no-op, regardless of source
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "", "user"))
	assert.Equal(t, "User Title", getTitle(t, f, chatID))
}

// TestRenameChat_BroadcastsTitleSetOnSuccessfulChange guards the title_set
// frame firing only on an actual persisted change, never on a no-op — the hub
// projection emits it off the SetTitle event, so a rename that returns before
// issuing SetTitle produces no frame.
func TestRenameChat_BroadcastsTitleSetOnSuccessfulChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset()

	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "A Title", "user"))
	assert.Equal(t, []string{"title_set"}, f.bcKinds(t))

	// An empty-title no-op must not broadcast again.
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "", "user"))
	assert.Equal(t, []string{"title_set"}, f.bcKinds(t))

	// A derived rename against an already-locked (user) title is also a no-op
	// and must not broadcast.
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Derived Attempt", "derived"))
	assert.Equal(t, []string{"title_set"}, f.bcKinds(t))
}

func TestRenameChat_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.RenameChat(ctx, "does-not-exist", "Some Title", "user")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename chat: get")
}

// TestRegression_RenameResolvesChatAtCallTime guards the fix for the stale-title
// bug: the OLD title instruction baked the chat id into the CLI's spawn-time
// system prompt, so an agent that titled itself after a /clear or /resume moved
// the runner to a NEW chat but still carried the OLD chat id — and renamed the
// chat it used to be in, not the one the user was looking at.
//
// RenameByRunner resolves runnerID → runner → CurrentChatID at CALL TIME, the
// same resolution IngestHook already uses for every hook (see its doc comment):
// nothing is baked in, so nothing can go stale. This spawns a runner on chat A,
// drives two conversation announcements — the second an unknown session id,
// which the context-move reducer (engine/agent.Decide) resolves to MoveToNew —
// so the runner ends up on a brand-new chat B while chat A still exists
// untouched, and proves the rename lands on B, never on A.
func TestRegression_RenameResolvesChatAtCallTime(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatA, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sA")
	f.announce(t, runnerID, "sB") // /clear — the runner moves to a NEW chat

	require.NoError(t, f.usecase.RenameByRunner(ctx, runnerID, "New Title", "agent"))

	moved := f.runner(t, runnerID)
	require.NotEqual(t, chatA, moved.CurrentChatID, "precondition: the runner really did move off chat A")

	assert.Equal(t, "New Title", getTitle(t, f, moved.CurrentChatID),
		"the title lands on the chat the runner is on NOW")
	assert.NotEqual(t, "New Title", getTitle(t, f, chatA),
		"and NOT on the chat it used to be on")
}

// TestRenameByRunner_UnknownRunner_ReturnsWrappedError guards the error path: a
// rename call naming a runner Crowbar has never heard of (or whose CLI has
// already exited) must fail loudly here — unlike a hook, which silently drops
// an unknown runner, this is a direct call the handler maps to a 404.
func TestRenameByRunner_UnknownRunner_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.RenameByRunner(ctx, "does-not-exist", "Some Title", "agent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename by runner")
}

// TestRenameByRunner_DisplacedRunner_IsANoop guards the one legitimate case
// where a runner's CurrentChatID is empty: it has been taken off its chat and
// is being retired (a switch, an eviction, a deleted chat) but its process has
// not yet died. There is nowhere to write the title, and — mirroring
// handleTurn's identical guard on the hook path — this must be a silent no-op,
// never an error that would otherwise reach the dying CLI's stderr for no
// actionable reason.
func TestRenameByRunner_DisplacedRunner_IsANoop(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.PurgeChat(ctx, mustChatOf(t, f, runnerID)))

	require.NoError(t, f.usecase.RenameByRunner(ctx, runnerID, "New Title", "agent"))
}

// mustChatOf reads back the chat id a freshly spawned runner is placed on.
func mustChatOf(t *testing.T, f testFixture, runnerID string) string {
	t.Helper()
	r := f.runner(t, runnerID)
	require.NotEmpty(t, r.CurrentChatID)
	return r.CurrentChatID
}

// TestIngestHook_UserPrompt_SetsDerivedTitle guards the first-prompt fallback
// wired into IngestHook's user_prompt case: deriveTitle(prompt) is applied via
// RenameChat(..., "derived"), which only fills an empty title.
func TestIngestHook_UserPrompt_SetsDerivedTitle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "Refactor the auth module to use JWT\nmore detail"})))
	assert.Equal(t, "Refactor the auth module to use JWT", getTitle(t, f, chatID))

	// a later prompt does not overwrite the derived title
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "now do something else"})))
	assert.Equal(t, "Refactor the auth module to use JWT", getTitle(t, f, chatID))
}

// ─── from purge_test.go ───────────────────────────────────────────────

// TestRegression_DeleteChat_KillsItsRunner
//
// A runner whose chat no longer exists is a runner with nowhere to write. The chat
// delete cascade must therefore get rid of it — but NOT by deleting its row from the
// read model. That would be Crowbar asserting a liveness fact it does not own (the
// process would still be running, invisible), which is the exact dual-authority
// mistake this model deletes. It kills the PROCESS and lets the row follow.
func TestRegression_DeleteChat_KillsItsRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	r := f.runner(t, runnerID)
	require.NotEmpty(t, r.TerminalSession)

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	assert.Contains(t, f.term.terminatedIDs(), r.TerminalSession,
		"the chat's CLI must be terminated, not orphaned")

	// The PTY dies, and THAT is what removes the runner.
	f.term.exit(t, r.TerminalSession)
	f.wait()

	_, err := f.runners.Get(f.ctx, runnerID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound, "no runner may point at a hard-deleted chat")
	_, err = f.runners.LiveRunnerForChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)
}

// TestRegression_DeleteChatMidTurn_DoesNotResurrectTheChat is the zombie-chat guard.
//
// Found in live daemon testing: a hard-deleted chat reappeared in the read model
// (list AND by-id GET) after deletion. The PTY teardown fires the runner-exit
// reconcile ASYNCHRONOUSLY, and that reconcile writes to the chat (it closes the turn
// the dead CLI left open); asynx projections are async, so a chat command that
// commits before the Forget can have its read-model Save land AFTER Forget's
// row-delete — re-creating the row.
//
// The old code fought this with an in-memory registry unbind the teardown checked
// first. That registry is gone, so the guard is now structural: the chat is Forgotten
// BEFORE the CLI is killed, and a Forgotten aggregate's event log is erased, so every
// later chat command fails Validate and emits nothing at all. Deleting mid-turn — the
// case that actually writes on exit — must leave the chat gone and staying gone.
func TestRegression_DeleteChatMidTurn_DoesNotResurrectTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "working on it"})))
	require.True(t, f.chat(t, chatID).Working, "precondition: the chat is mid-turn when it is deleted")

	termSess := f.runner(t, runnerID).TerminalSession
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	// ...and now the CLI actually dies, off the reap goroutine, as it does in production.
	f.term.exit(t, termSess)
	f.wait()

	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound, "a deleted chat must not come back as a zombie row")

	chats, err := f.usecase.ListChats(f.ctx)
	require.NoError(t, err)
	for _, c := range chats {
		assert.NotEqual(t, chatID, c.ID, "a forgotten chat must not reappear in ListChats")
	}
}

// TestPurgeChat_ForgetsTheChatOutright: Forget erases the aggregate — event log and
// read-model row — so a genuinely gone chat 404s even a by-id lookup.
func TestPurgeChat_ForgetsTheChatOutright(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	_, err := f.usecase.GetChat(f.ctx, chatID)
	require.Error(t, err, "a forgotten chat must not be readable even by direct GetChat")
	assert.ErrorIs(t, err, agentchat.ErrNotFound)

	chats, err := f.usecase.ListChats(f.ctx)
	require.NoError(t, err)
	for _, c := range chats {
		assert.NotEqual(t, chatID, c.ID, "a forgotten chat must not appear in ListChats")
	}
}

// TestPurgeChat_DropsConversationHistory: conversation history is append-only and
// outlives the process, so nothing else ever removes it. A conversation left pointing
// at a hard-deleted chat is a live trap — a later /resume of that id would resolve to
// a chat that no longer exists, and the runner would be moved somewhere unreachable.
func TestPurgeChat_DropsConversationHistory(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-1")
	require.Equal(t, chatID, f.chatForSession(t, "sid-1"), "precondition: the conversation is known")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	f.wait()

	_, err := f.runners.ChatForSession(f.ctx, "ws1", "sid-1")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound,
		"the deleted chat's conversations must not keep resolving to it")
	_, err = f.runners.LastConversation(f.ctx, chatID)
	assert.ErrorIs(t, err, agentrunner.ErrNotFound)
}

// TestPurgeChat_DormantChat_NothingToTerminate: a chat no runner points at has no CLI
// to kill, and that absence is an answer, not an error.
func TestPurgeChat_DormantChat_NothingToTerminate(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession) // the CLI exits on its own
	f.wait()

	before := len(f.term.terminateRequestIDs())
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	assert.Len(t, f.term.terminateRequestIDs(), before, "a dormant chat has no CLI to terminate")
	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

// TestPurgeChat_TerminateFailure_SessionAlreadyGone_ContinuesPurge covers the purge's
// tolerance for a terminal session that is already gone (the one error the real
// terminal engine returns today).
func TestPurgeChat_TerminateFailure_SessionAlreadyGone_ContinuesPurge(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.term.terminateErr = fmt.Errorf("terminal: terminate: %w: term-1", engineterminal.ErrSessionNotFound)

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

// TestPurgeChat_TerminateFailure_OtherError_IsBestEffort_StillPurges: a genuine
// TerminateGraceful failure must NOT abort the purge. An orphaned PTY is a far smaller
// harm than a chat the user can never remove.
func TestPurgeChat_TerminateFailure_OtherError_IsBestEffort_StillPurges(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	termSess := f.runner(t, runnerID).TerminalSession
	f.term.terminateErr = errors.New("boom: terminate genuinely failed")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID),
		"a genuine terminate failure must not abort the purge (best-effort)")

	assert.Contains(t, f.term.terminateRequestIDs(), termSess)
	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

// TestPurgeChat_ReapsChatDirOnDisk: a standalone hard delete must remove the chat's
// PLAINTEXT on-disk footprint (its handoff ledger), not only Forget the aggregate.
//
// It does NOT remove the runner's tmp dir, which is not the chat's to remove: that dir is
// the config of a PROCESS that is still alive (we have asked it to quit, and a SIGTERM is
// not synchronous), and it is keyed by the runner rather than the chat precisely so that it
// can be reaped when the process actually dies — or, if the daemon dies first, at the next
// boot (worktreepath.RunnerDir).
func TestPurgeChat_ReapsChatDirOnDisk(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	chatDir := filepath.Join(f.ws.chatsDir, chatID)
	require.NoError(t, os.MkdirAll(chatDir, 0o700))
	turn(t, f, runnerID, "claude", "a turn, so the chat has a conversation to purge")

	before, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.NotEmpty(t, before, "precondition: the chat has a recorded conversation")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	after, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, after, "purge must drop the chat's conversation record")

	_, err = os.Stat(chatDir)
	assert.True(t, os.IsNotExist(err), "purge must reap the chat's on-disk dir")
}

// TestPurgeChat_ReapFailure_StillPurges: the on-disk reap is best-effort — even if the
// chat dir cannot be resolved, the aggregate is still Forgotten and PurgeChat returns
// nil rather than failing a delete the user asked for.
func TestPurgeChat_ReapFailure_StillPurges(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.ws.err = errors.New("boom: workspace lookup for reap")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID), "a reap-path failure must not abort the purge")

	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound, "the aggregate is still Forgotten")
}

// TestPurgeChat_ReapRefusesChatsDirOutsideHome pins the removal-site backstop: if
// AgentChatsDir ever resolves a chats dir OUTSIDE crowbar home — the scenario a crafted
// repo RemoteSlug containing "../" creates, since filepath.Join collapses ".." and can
// escape home — the hard-delete reap must REFUSE the os.RemoveAll rather than delete a
// path on the user's real filesystem.
func TestPurgeChat_ReapRefusesChatsDirOutsideHome(t *testing.T) {
	f := newFixture(t)

	// Stand in for a chats dir that escaped home (what a "../"-poisoned slug would
	// yield): a directory that is NOT under f.ws.home.
	escaped := t.TempDir()
	f.ws.chatsDir = escaped

	chatID, _ := f.spawn(t, "claude")

	sentinel := filepath.Join(escaped, chatID, "sentinel")
	require.NoError(t, os.MkdirAll(filepath.Dir(sentinel), 0o755))
	require.NoError(t, os.WriteFile(sentinel, []byte("x"), 0o600))

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	_, statErr := os.Stat(sentinel)
	assert.NoError(t, statErr,
		"a chats dir outside crowbar home must NEVER be removed by the purge reap")
}

// TestPurgeChat_UnknownChat_ReturnsWrappedError: PurgeChat on an id with no chat wraps
// the lookup failure rather than panicking or silently no-oping.
func TestPurgeChat_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.PurgeChat(f.ctx, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "purge chat: get")
}

// ─── from chatlog_test.go ─────────────────────────────────────────────

// TestReadChatLog_RendersTheLedger guards agent.ChatUsecase.ReadChatLog — the
// production agenttools.ChatLogReader get_chat_log calls once a chat's
// workspace has already cleared the caller's CanSee check. Unlike
// AssembleHandoff it carries the RAW conversation with no preamble/footer
// wrapper: get_chat_log hands prose straight to a model, not an injected
// spawn-time context document.
//
// It reports TURNS, not finished text, because get_chat_log caps what it returns
// and states how many turns it dropped — a count that can only be taken where
// turns are still separate values. The speaker attribution still has to be the
// ledger's own ("assistant (<provider>)"), since that is the wording every chat
// log Crowbar has produced already uses.
func TestReadChatLog_RendersTheLedger(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "s1", "last_assistant_message": "done thing"})))

	out, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, out, 1)
	assert.Equal(t, "assistant (claude)", out[0].Speaker)
	assert.Equal(t, "done thing", out[0].Body)
}

// An unspoken chat's ledger is empty. ReadChatLog itself returns "" (not an
// error) rather than agenttools.NoChatTurnsText: turning that into explicit
// "no turns" prose is getChatLog's job (the tool layer), the single place that
// normalization lives, since get_chat_log is ReadChatLog's only caller today —
// see TestGetChatLog_EmptyLedgerIsExplicitNotAnError in the agenttools package
// for the tool-layer half of this contract.
func TestReadChatLog_EmptyLedgerReturnsEmptyNotAnError(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	out, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestReadChatLog_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.ReadChatLog(f.ctx, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read chat log")
}

// ─── from thread_test.go ──────────────────────────────────────────────

// injectedContext returns the {context} document the Nth spawn was given.
// claude's channel is --append-system-prompt, a silent flag whose VALUE is the
// whole document — so what the CLI was told is read straight off the argv the
// spawn built, not inferred from anything Crowbar kept.
func injectedContext(
	t *testing.T,
	f testFixture,
	spawn int,
) string {
	t.Helper()
	argv := f.term.calls[spawn].argv
	at := indexOf(argv, "--append-system-prompt")
	require.GreaterOrEqual(t, at, 0, "spawn %d carried no context channel", spawn)
	require.Less(t, at+1, len(argv), "--append-system-prompt with no value")
	return argv[at+1]
}

// thread makes chatID a thread of parentID, the way a drag in the Chats panel
// does: the placement command on the chat aggregate, and nothing else. Nothing
// is copied and no ledger is touched — the parent edge IS the whole record.
func thread(
	t *testing.T,
	f testFixture,
	chatID string,
	parentID string,
) {
	t.Helper()
	_, err := f.chats.SetPlacement(f.ctx, chatID, parentID, 0)
	require.NoError(t, err)
	f.wait()
}

// file puts a folder row in the workspace's chat-folder table.
func file(
	t *testing.T,
	f testFixture,
	id string,
	parentID string,
) {
	t.Helper()
	require.NoError(t, f.folders.Save(f.ctx, domain.ChatFolder{
		ID: id, WorkspaceID: "ws1", ParentID: parentID, Name: id,
	}))
}

// lineageBlock returns the configured thread_lineage prompt with the ids filled
// in, so these tests track the config-driven text rather than re-hardcoding it.
func lineageBlock(
	ids ...string,
) string {
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, "- "+id)
	}
	return strings.ReplaceAll(config.GetPrompts().ThreadLineage, "{lineage}", strings.Join(lines, "\n"))
}

// A thread is SPAWNED knowing what it reads. Not handed the turns — handed the
// ids and told to go and get them, which is what makes the relationship live:
// the parent's later turns are in the answer too.
func TestSpawn_AThreadIsPointedAtTheChatItHangsOff(t *testing.T) {
	f := newFixture(t)

	parentID, parentRunner := f.spawn(t, "claude")
	turn(t, f, parentRunner, "claude", "the parent worked out the plan here")
	threadID, _ := f.spawn(t, "claude")
	thread(t, f, threadID, parentID)

	// Any later spawn on that chat: a provider switch is the shortest one to drive.
	_, err := f.usecase.SwitchProvider(f.ctx, threadID, "claude")
	require.NoError(t, err)
	f.wait()

	got := injectedContext(t, f, 2)
	assert.Contains(t, got, lineageBlock(parentID))
	assert.Contains(t, got, "get_chat_log")
	assert.NotContains(t, got, "the parent worked out the plan here",
		"a POINTER, never a paste: pasting the parent's turns would freeze it at this instant")
}

// Folders are transparent, proved where it counts — at the spawn, against the
// document the CLI is actually given. A thread two folders deep under a chat is
// told exactly what one sitting directly under it is told.
func TestSpawn_FoldersDoNotChangeWhatAThreadIsTold(t *testing.T) {
	f := newFixture(t)

	parentID, _ := f.spawn(t, "claude")
	directID, _ := f.spawn(t, "claude")
	filedID, _ := f.spawn(t, "claude")
	file(t, f, "outer", parentID)
	file(t, f, "inner", "outer")
	thread(t, f, directID, parentID)
	thread(t, f, filedID, "inner")

	_, err := f.usecase.SwitchProvider(f.ctx, directID, "claude")
	require.NoError(t, err)
	f.wait()
	_, err = f.usecase.SwitchProvider(f.ctx, filedID, "claude")
	require.NoError(t, err)
	f.wait()

	assert.Contains(t, injectedContext(t, f, 3), lineageBlock(parentID))
	assert.Contains(t, injectedContext(t, f, 4), lineageBlock(parentID),
		"filing a thread away is organisation; it must not change one word of what it reads")
}

// Nearest parent first, because that is the order the ids are useful in.
func TestSpawn_ADeepThreadIsToldItsWholeChainNearestFirst(t *testing.T) {
	f := newFixture(t)

	grandparentID, _ := f.spawn(t, "claude")
	parentID, _ := f.spawn(t, "claude")
	threadID, _ := f.spawn(t, "claude")
	thread(t, f, parentID, grandparentID)
	thread(t, f, threadID, parentID)

	_, err := f.usecase.SwitchProvider(f.ctx, threadID, "claude")
	require.NoError(t, err)
	f.wait()

	assert.Contains(t, injectedContext(t, f, 3), lineageBlock(parentID, grandparentID))
}

// A chat with no chat ancestors must behave exactly as it does today — not
// "almost", exactly, which is why this compares against a chat that has never
// been near the tree rather than against a substring.
func TestSpawn_AChatWithNoChatAncestorsIsToldNothingExtra(t *testing.T) {
	f := newFixture(t)

	plainID, _ := f.spawn(t, "claude")
	filedID, _ := f.spawn(t, "claude")
	file(t, f, "somewhere", "")
	thread(t, f, filedID, "somewhere")

	_, err := f.usecase.SwitchProvider(f.ctx, plainID, "claude")
	require.NoError(t, err)
	f.wait()
	_, err = f.usecase.SwitchProvider(f.ctx, filedID, "claude")
	require.NoError(t, err)
	f.wait()

	assert.Equal(t, injectedContext(t, f, 2), injectedContext(t, f, 3),
		"a chat merely filed in a folder inherits nothing and is spawned identically to an unfiled one")
	assert.NotContains(t, injectedContext(t, f, 3), "THREAD CONTEXT")
}

// A lineage that cannot be read fails the spawn. A thread that comes up silently
// believing itself standalone would then do the whole task without the context
// it exists to continue, and nothing anywhere would say so.
func TestSpawn_ALineageThatCannotBeReadFailsTheSpawn(t *testing.T) {
	f := newFixture(t)

	parentID, _ := f.spawn(t, "claude")
	threadID, _ := f.spawn(t, "claude")
	thread(t, f, threadID, parentID)
	f.folders.FindErr = errors.New("folder table unreadable")

	_, err := f.usecase.SwitchProvider(f.ctx, threadID, "claude")
	require.ErrorContains(t, err, "folder table unreadable")
	require.Len(t, f.term.calls, 2, "and no CLI was started")
}

// The chat a spawn is MINTING cannot have been placed anywhere yet — the
// aggregate is written after the CLI is live — so that spawn resolves no lineage
// at all rather than failing on a chat that does not exist.
func TestSpawn_MintingAChatResolvesNoLineage(t *testing.T) {
	f := newFixture(t)
	f.folders.FindErr = errors.New("folder table unreadable")

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// The move is written down
// ---------------------------------------------------------------------------

// Re-parenting takes effect from the move ONWARD, and the record of that says so
// at the point in the conversation where it happened. A retroactive rewrite of
// what a chat has read is the version nobody can audit afterwards.
func TestNoteThreadLineage_IsAppendedToTheChatsOwnConversation(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	turn(t, f, runnerID, "claude", "fifty turns of its own, had without any of this")

	require.NoError(t, f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1", "root-1"}))

	turns, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, turns, 2)
	assert.Contains(t, turns[0].Body, "fifty turns of its own")
	assert.Contains(t, turns[1].Body, "parent-1, root-1")
	assert.Contains(t, turns[1].Body, "[Crowbar]")
	assert.Contains(t, turns[1].Body, "from this point on",
		"the record has to date the change, or it reads as though the chat always had this context")
}

func TestNoteThreadLineage_UnknownChat_ReturnsError(t *testing.T) {
	f := newFixture(t)

	require.Error(t, f.usecase.NoteThreadLineage(f.ctx, "no-such-chat", []string{"parent-1"}))
}

// The note is tagged as Crowbar's own, never as a provider's. Ledger.LastTurnAt
// decides whether a provider has a conversation on disk worth resuming, and a
// note wearing a provider's name would answer yes on its behalf for a session it
// never held — the exact confusion that once killed a chat on --resume.
func TestNoteThreadLineage_IsNotAttributedToAnyProvider(t *testing.T) {
	f := newFixture(t)

	// claude opens a conversation and then says NOTHING in it. Only a turn tagged
	// "claude" would make that conversation resumable — and the note below is the
	// only turn this ledger is ever going to have.
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude")
	require.NoError(t, f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1"}))

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	assert.Equal(t, -1, indexOf(f.term.calls[2].argv, "--resume"),
		"a Crowbar note is not a claude turn: resuming a session claude never spoke in is what "+
			"claude refuses with \"No conversation found\", and it once killed a chat outright")
}

// ---------------------------------------------------------------------------
// The two halves of a placed create
// ---------------------------------------------------------------------------

// MintChat writes the chat and starts nothing. The chat that comes back is a
// DORMANT one — the state the panel already models — so the caller is free to
// place it before deciding to start anything on it.
func TestMintChat_CreatesTheChatAndNoRunner(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "ws1")
	require.NoError(t, err)
	f.wait()

	chat, err := f.usecase.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, "ws1", chat.WorkspaceID)
	assert.Empty(t, f.term.calls, "minting a chat starts no CLI")
	_, err = f.liveRunnerFor(t, chatID)
	assert.Error(t, err, "and places no runner on it")
}

func TestMintChat_SurfacesACreateFailure(t *testing.T) {
	f, chatStore, _ := newFaultFixture(t)
	chatStore.failCreate = errors.New("chat store down")

	_, err := f.usecase.MintChat(f.ctx, "ws1")
	require.ErrorContains(t, err, "chat store down")
}

// The regression this whole path exists for, at the unit level: a chat that is
// ALREADY a thread when its first CLI is started is told so on that first
// session. StartRunner takes the ordinary create=false spawn, so nothing
// special-cases it — it falls out of the chat existing and being placed first.
func TestStartRunner_ATheadPlacedBeforeItsFirstSpawnIsToldItsLineage(t *testing.T) {
	f := newFixture(t)

	parentID, _ := f.spawn(t, "claude")
	threadID, err := f.usecase.MintChat(f.ctx, "ws1")
	require.NoError(t, err)
	f.wait()
	thread(t, f, threadID, parentID)

	runnerID, err := f.usecase.StartRunner(f.ctx, threadID, "claude")
	require.NoError(t, err)
	require.NotEmpty(t, runnerID)
	f.wait()

	assert.Contains(t, injectedContext(t, f, 1), lineageBlock(parentID),
		"a thread placed before its first spawn must be told what it reads ON that first spawn")
}

// A chat that is not a thread is started exactly as it always was.
func TestStartRunner_AnUnplacedChatIsToldNothingExtra(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "ws1")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.StartRunner(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	assert.NotContains(t, injectedContext(t, f, 0), "THREAD CONTEXT")
}

// The workspace comes from the CHAT, so an unknown chat is refused before any
// process is started rather than spawning a CLI against a workspace nobody named.
func TestStartRunner_UnknownChat_StartsNothing(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.StartRunner(f.ctx, "no-such-chat", "claude")
	require.Error(t, err)
	assert.Empty(t, f.term.calls)
}

// A chat that has said nothing gets no move note. There is nothing above the line
// for "everything above this line" to refer to, and a chat BORN under a parent is
// told its lineage at spawn rather than reading about a move it never experienced.
func TestNoteThreadLineage_SaysNothingInAChatThatHasNotSpoken(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "ws1")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1"}))

	turns, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, turns, "a chat with no history has no move to date")
}

// A record that cannot be READ is not an empty one, so it must not be treated as
// the "nothing to date" case — the note would be silently dropped for a chat that
// has a whole history.
func TestNoteThreadLineage_SurfacesAnUnreadableRecord(t *testing.T) {
	f, activity := newActivityFaultFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	turn(t, f, runnerID, "claude", "something was said here")
	activity.turnsErr = errors.New("record unreadable")

	require.ErrorContains(t,
		f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1"}), "record unreadable")
}

// The same regression one layer up, through the REAL spawn: a chat that is
// already placed must have its lineage injected even while the read model still
// reports it unplaced.
//
// The projection is forced stale rather than raced. Live, the window between a
// placement being written and its projection folding is microseconds wide, so a
// test that merely places and spawns passes about half the time — which is how a
// spawn that resolved lineage from the read model, and therefore threaded
// nothing, survived a whole suite. Holding the window open makes the property
// testable at all: a spawn must not decide what a chat inherits on projected
// state, whatever the projection happens to say.
func TestStartRunner_ThreadsAChatTheReadModelStillReportsUnplaced(t *testing.T) {
	f, chatStore, _ := newFaultFixture(t)

	parentID, _ := f.spawn(t, "claude")
	threadID, err := f.usecase.MintChat(f.ctx, "ws1")
	require.NoError(t, err)
	f.wait()
	thread(t, f, threadID, parentID)

	// Everything the projection could have folded, it has. What it reports from
	// here is a deliberate lie, and the only truthful source left is the log.
	chatStore.staleProjection = true

	_, err = f.usecase.StartRunner(f.ctx, threadID, "claude")
	require.NoError(t, err)
	f.wait()

	assert.Contains(t, injectedContext(t, f, 1), lineageBlock(parentID),
		"the spawn must resolve placement from the event log, never from the projection it just outran")
}

// ─── from handoff_test.go ─────────────────────────────────────────────

func TestAssembleHandoff_WrapsLedgerEntriesInPreamble(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	turn(t, f, runnerID, "claude", "first turn transcript")
	turn(t, f, runnerID, "claude", "second turn transcript")

	got, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)

	// AssembleHandoff wraps the rendered ledger in the CONFIGURED handoff_wrapper
	// (config-driven, not a hardcoded literal): assert against the actual configured
	// template split around {conversation}, so this test tracks config-driven behavior
	// rather than re-hardcoding it.
	wrapper := config.GetPrompts().HandoffWrapper
	pre, post, ok := strings.Cut(wrapper, "{conversation}")
	require.True(t, ok, "handoff_wrapper must contain {conversation}")
	assert.True(t, strings.HasPrefix(got, pre))
	assert.True(t, strings.HasSuffix(got, post))
	assert.Contains(t, got, "first turn transcript")
	assert.Contains(t, got, "second turn transcript")
	// Both entries must appear, in append order.
	assert.Less(t, strings.Index(got, "first turn transcript"), strings.Index(got, "second turn transcript"))
}

func TestAssembleHandoff_EmptyLedger_ReturnsEmptyString(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	got, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, got)
}

func TestAssembleHandoff_UnknownChat_ReturnsError(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.AssembleHandoff(f.ctx, "does-not-exist")
	require.Error(t, err)
}

// resumeClaudeWithGap drives the one path where Crowbar's own context document comes
// back at it as a user prompt: claude switched away and then BACK. A resumed
// hooks-transport CLI cannot be reached through any config channel (verified against
// 0.139.0), so the gap is delivered as a positional — which IS claude's first user
// message, and which its user-prompt hook duly reports. Returns the new claude runner
// and the exact document it was spawned with.
//
// claude, not codex: codex is api-transport and non-hotswap, so ITS OWN resume happens
// over the api connection (applyAPITransport's thread/resume), and the redundant
// hooks-only PTY spawnRunner still forks alongside it must never ALSO be handed this
// pointer — apiOwnsResume (prompts.go) withholds it there for exactly that reason (a
// second, disconnected "codex" conversation would otherwise answer it, confirmed live).
// claude has no such competing connection: its PTY IS the conversation, so this
// mechanism is still its live, correct delivery path.
func resumeClaudeWithGap(t *testing.T, f testFixture) (chatID, claudeRunnerID, injected string) {
	t.Helper()

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude")
	turn(t, f, claudeRunner, "claude", "claude said this before leaving")

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	// codex's turn_stop maps threadId/turn.items[type=agentMessage].text (see
	// codex.yaml), NOT the flat last_assistant_message shape turn() builds for
	// claude — using turn() here would silently extract an empty message.
	f.announce(t, codexRunner, "sid-codex-away")
	require.NoError(t, f.usecase.IngestHook(f.ctx, codexRunner, "codex", "turn_stop",
		mustJSON(t, map[string]any{
			"threadId": "sid-codex-away",
			"turn": map[string]any{
				"items": []any{
					map[string]any{"type": "agentMessage", "text": "codex spoke while claude was away"},
				},
			},
		})))
	f.wait()

	claudeRunnerID, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	argv := f.term.calls[2].argv
	injected = argv[len(argv)-1]

	// The real gap, not a pointer asking the model to go fetch it: a
	// get_chat_log pointer relies on the model choosing to call a tool from a
	// context reminder, and that bet did not hold up live — claude regularly
	// never called it (see claude.yaml's resume_context_inject for the fuller
	// account). Handing the actual conversation removes that dependency.
	require.Contains(t, injected, "codex spoke while claude was away",
		"the resumed claude must be handed the real gap, not a pointer to fetch later: %v", argv)
	require.NotContains(t, injected, "claude said this before leaving",
		"a provider resumed into its own conversation must not be re-fed its own earlier turns")
	require.True(t, strings.HasPrefix(injected, "<system-reminder>") && strings.HasSuffix(injected, "</system-reminder>"),
		"the gap must be wrapped as trusted context, not delivered as a bare user turn: %q", injected)
	return chatID, claudeRunnerID, injected
}

// TestResumeClaude_InjectedPointer_IsNotRecordedAsAUserTurn: the pointer Crowbar hands a
// resumed claude comes straight back through its user-prompt hook — that is Crowbar's own
// message echoing, not something the user said. Recording it would put Crowbar's
// plumbing into the conversation the next handoff is built from.
func TestResumeClaude_InjectedPointer_IsNotRecordedAsAUserTurn(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner, injected := resumeClaudeWithGap(t, f)

	require.NoError(t, f.usecase.IngestHook(f.ctx, claudeRunner, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": injected})))
	f.wait()

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)

	assert.NotContains(t, handoff, "WHILE YOU WERE AWAY",
		"Crowbar's injected context document must never be recorded as a user turn:\n%s", handoff)
	assert.Contains(t, handoff, "codex spoke while claude was away", "the real conversation is still recorded")
	assert.Contains(t, handoff, "claude said this before leaving")
}

// TestResumeClaude_InjectedPointer_StillOpensTheTurn: suppressing the echo from the
// LEDGER must not suppress the turn itself — the CLI really is answering it, so the chat
// must read as Working (the workspace spinner overlay depends on it).
func TestResumeClaude_InjectedPointer_StillOpensTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner, injected := resumeClaudeWithGap(t, f)

	require.NoError(t, f.usecase.IngestHook(f.ctx, claudeRunner, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": injected})))
	f.wait()

	assert.True(t, f.chat(t, chatID).Working, "the CLI is answering the gap: the chat must read as working")
}

// TestResumeClaude_UserRetypesThePointer_IsRecorded: the suppression is one-shot and
// scoped to the runner the message was injected into, so a user who genuinely sends that
// same text later is still recorded — the guard must never become a permanent content
// filter.
func TestResumeClaude_UserRetypesThePointer_IsRecorded(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner, injected := resumeClaudeWithGap(t, f)

	for range 2 {
		require.NoError(t, f.usecase.IngestHook(f.ctx, claudeRunner, "claude", "user_prompt",
			mustJSON(t, map[string]any{"prompt": injected})))
		f.wait()
	}

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, 1, strings.Count(handoff, "WHILE YOU WERE AWAY"),
		"the FIRST echo is dropped; a second, genuinely user-sent copy is recorded:\n%s", handoff)
}

// TestRunnerExit_ForgetsTheInjectedContext: the echo guard is per-spawn state about a
// LIVE process. Once the PTY is gone the entry means nothing and must not accumulate —
// it holds a whole handoff document, and a long-lived daemon spawns a lot of CLIs.
func TestRunnerExit_ForgetsTheInjectedContext(t *testing.T) {
	f := newFixture(t)

	_, claudeRunner, injected := resumeClaudeWithGap(t, f)

	// Precondition: the guard is armed — Crowbar's own document is recognised as an echo.
	require.True(t, f.engine.WasInjected(claudeRunner, injected))

	// Re-arm it (the match above consumed it), then kill the PTY.
	f.engine.RecordInjection(claudeRunner, injected)
	f.term.exit(t, f.runner(t, claudeRunner).TerminalSession)
	f.wait()

	assert.False(t, f.engine.WasInjected(claudeRunner, injected),
		"a dead runner's injected context must be forgotten")
}

// ─── from selection_test.go ───────────────────────────────────────────

func writeDescriptor(t *testing.T, f testFixture, id, body string) {
	t.Helper()
	dir := filepath.Join(f.ws.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(body), 0o600))
}

const selectingDescriptorBody = `
id: claude
display_name: Selecting
spawn:
  cmd: claude
  interactive_required: true
  forbid_flags: ["-p", "--print"]
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
model:
  available: [sonnet, opus]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
effort:
  available:
    "*": [low, high]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--effort", value: "{effort}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

const silentDescriptorBody = `
id: claude
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
presentation:
  prompt_submit:
    strategy: restart_tui
    fresh:
      - pass_arg: { positional: "{message}" }
    resume:
      - pass_arg: { positional: "{message}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

func TestSetChatSelection_WritesADeclaredChoice(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))

	chat := f.chat(t, chatID)
	assert.Equal(t, "opus", chat.Model)
	assert.Equal(t, "high", chat.Effort)
}

func TestSetChatSelection_ClearsBackToTheProviderDefault(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))

	chat := f.chat(t, chatID)
	assert.Empty(t, chat.Model)
	assert.Empty(t, chat.Effort)
}

func TestSetChatSelection_RefusesAValueOutsideTheDeclaredCatalogue(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	testCases := []struct {
		name    string
		model   string
		effort  string
		wantMsg string
	}{
		{"unknown model", "gpt-5", "", "declares no model"},
		{"unknown effort", "opus", "ludicrous", "declares no effort"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := f.usecase.SetChatSelection(f.ctx, chatID, tc.model, tc.effort)

			require.ErrorIs(t, err, apperr.ErrInvalidArgument)
			assert.Contains(t, err.Error(), tc.wantMsg)
			assert.Empty(t, f.chat(t, chatID).Model, "a refused write must store nothing")
		})
	}
}

func TestSetChatSelection_RefusesWhereTheProviderDeclaresNoCatalogue(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", silentDescriptorBody)
	chatID, _ := f.spawn(t, "claude")

	err := f.usecase.SetChatSelection(f.ctx, chatID, "gpt-5", "")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestSetChatSelection_UnknownChatIsNotFound(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.SetChatSelection(f.ctx, "no-such-chat", "opus", "")

	require.Error(t, err)
}

func TestSetChatSelection_ValidatesTheEffortAgainstTheIncomingModel(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", `
id: claude
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
model:
  available: [sonnet, opus]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--model", value: "{model}" }
effort:
  available:
    "*": [low]
    opus: [max]
  strategy: restart_tui
  apply:
    - pass_arg: { arg: "--effort", value: "{effort}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "max"))
	err := f.usecase.SetChatSelection(f.ctx, chatID, "sonnet", "max")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

func TestSpawn_UnselectedChatSpawnsIdenticalArgv(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	baseline := f.term.calls[0].argv

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello", uuid.NewString())
	require.NoError(t, err)

	replacement := f.term.calls[f.term.callCount()-1].argv
	for _, arg := range replacement {
		assert.NotEqual(t, "--model", arg)
		assert.NotEqual(t, "--effort", arg)
	}

	require.GreaterOrEqual(t, len(replacement), len(baseline))
	assert.Equal(t, baseline[:2], replacement[:2])
}

func TestSpawn_CarriesTheSelectionIntoTheArgvAndRecordsItOnTheRunner(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "with a model", uuid.NewString())
	require.NoError(t, err)

	call := f.term.calls[f.term.callCount()-1]
	modelAt := indexOf(call.argv, "--model")
	require.GreaterOrEqual(t, modelAt, 0)
	assert.Equal(t, "opus", call.argv[modelAt+1])
	effortAt := indexOf(call.argv, "--effort")
	require.GreaterOrEqual(t, effortAt, 0)
	assert.Equal(t, "high", call.argv[effortAt+1])

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "opus", live.LaunchModel)
	assert.Equal(t, "high", live.LaunchEffort)
}

type undeliverableAgent struct {
	engineagents.Agent
}

func (undeliverableAgent) Capabilities() engineagents.Capabilities {
	return engineagents.Capabilities{PromptSubmit: true, Delivery: "telepathy"}
}

func TestSubmitPrompt_ASelectionSwitchAuthorisesARestartADeliveryWouldNotHaveDone(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	shipped, err := f.engine.Get(f.ctx, f.ws.home, "claude")
	require.NoError(t, err)
	descriptor := undeliverableAgent{Agent: shipped}
	require.NotEqual(t, engineagents.DeliveryRestartTUI, descriptor.Capabilities().Delivery,
		"the fixture must not restart for delivery reasons, or this proves nothing")

	err = agentusecase.RequirePromptRestart(f.ctx, f.usecase.RunnerUsecase, chatID, live, descriptor)
	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported,
		"a delivery this daemon has no channel for is refused, never guessed at")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))

	require.NoError(t, agentusecase.RequirePromptRestart(f.ctx, f.usecase.RunnerUsecase, chatID, live, descriptor),
		"a pending selection switch obliges a restart on its own")
}

func TestSubmitPrompt_TheRestartResumesTheNativeConversation(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")
	turn(t, f, runnerID, "claude", "the conversation exists")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "keep my history", uuid.NewString())

	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	resumeAt := indexOf(call.argv, "--resume")
	require.GreaterOrEqual(t, resumeAt, 0, "the forced restart must resume, not start fresh")
	assert.Equal(t, "native-session", call.argv[resumeAt+1])
}

func TestSubmitPrompt_ClearingBackToTheDefaultAlsoForcesTheRestart(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))
	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "under opus", uuid.NewString())
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(f.ctx, first.RunnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "under opus"})))
	turn(t, f, first.RunnerID, "claude", "answered")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "back to default", uuid.NewString())

	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	assert.Less(t, indexOf(call.argv, "--model"), 0, "the default carries no model flag")
}

func TestSubmitPrompt_NoSwitchUnderARestartingDeliveryIsUnchanged(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "ordinary message", uuid.NewString())

	require.NoError(t, err)
	assert.Equal(t, 2, f.term.callCount(), "the original spawn plus the delivery restart")
}

func TestResolveProviders_PublishesTheCatalogueAndItsAbsence(t *testing.T) {
	f := newFixture(t)

	list, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	byID := map[string]int{}
	for i, p := range list {
		byID[p.ID] = i
	}
	claude := list[byID["claude"]]
	assert.True(t, claude.ModelSelect)
	assert.True(t, claude.EffortSelect)
	assert.Equal(t, []string{"sonnet", "opus", "haiku"}, claude.Models)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, claude.Efforts[""])
	for _, model := range claude.Models {
		assert.NotEmpty(t, claude.Efforts[model], "every selectable model must answer its levels")
	}

	codex := list[byID["codex"]]
	assert.True(t, codex.ModelSelect)
	assert.True(t, codex.EffortSelect)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max", "ultra"},
		codex.Efforts["gpt-5.6-sol"])
	assert.Equal(t, []string{"low", "medium", "high", "xhigh"}, codex.Efforts["gpt-5.4"])
	assert.NotContains(t, codex.Efforts, "")
}

func TestResolveProviders_AProviderDeclaringNothingReportsNoCatalogue(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", silentDescriptorBody)

	list, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	for _, p := range list {
		if p.ID != "claude" {
			continue
		}
		assert.False(t, p.ModelSelect)
		assert.False(t, p.EffortSelect)
		assert.Empty(t, p.Models)
		assert.Nil(t, p.Efforts)
		return
	}
	t.Fatal("the overridden provider must still be listed")
}

func TestSetChatSelection_ADormantChatIsJudgedByItsLastProvider(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")
	require.NoError(t, f.usecase.StopChat(f.ctx, chatID))
	f.wait()
	_, err := f.liveRunnerFor(t, chatID)
	require.Error(t, err, "the chat must be dormant for this to prove anything")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))

	assert.Equal(t, "opus", f.chat(t, chatID).Model)
}

func TestSetChatSelection_AChatNoProviderHasEverRunOnIsUnprocessable(t *testing.T) {
	f := newFixture(t)
	chatID, err := f.usecase.MintChat(f.ctx, "ws1")
	require.NoError(t, err)
	f.wait()

	err = f.usecase.SetChatSelection(f.ctx, chatID, "opus", "")

	require.ErrorIs(t, err, apperr.ErrUnprocessable)

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
}

func TestSetChatSelection_SaveFailureSurfaces(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	cs.failSetSelection = errors.New("boom: save selection")

	err := f.usecase.SetChatSelection(f.ctx, chatID, "opus", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "save")
}

func TestSpawn_AnUnreadableSelectionFailsBeforeTheFork(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	spawnsBefore := f.term.callCount()

	cs.failLoadChat = errors.New("boom: load chat")
	cs.failLoadChatAfter = 1

	_, err := f.usecase.StartRunner(f.ctx, chatID, "claude")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat selection")
	assert.Equal(t, spawnsBefore, f.term.callCount(), "no process may exist after this failure")
}

func TestSubmitPrompt_ATerminalOnlyProviderIsUnsupported(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", `
id: claude
spawn:
  cmd: claude
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  turn_stop:
    in: turn_stop
    map:
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "nowhere to go", uuid.NewString())

	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported)
}

func TestSubmitPrompt_AnUnreadableSelectionRefusesTheDelivery(t *testing.T) {
	f, cs, _ := newFaultFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	shipped, err := f.engine.Get(f.ctx, f.ws.home, "claude")
	require.NoError(t, err)
	cs.failLoadChat = errors.New("boom: load chat")

	err = agentusecase.RequirePromptRestart(
		f.ctx, f.usecase.RunnerUsecase, chatID, live, undeliverableAgent{Agent: shipped},
	)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "chat selection")
}

// ─── from displace_test.go ────────────────────────────────────────────

// ---------------------------------------------------------------------------
// A runner must never be left pointed at a chat that no longer exists.
// ---------------------------------------------------------------------------

// TestRegression_SessionStartAfterItsChatWasDeleted_IsDroppedNotRecorded
//
// PurgeChat SIGTERMs the chat's CLI, but a SIGTERM is not synchronous — the CLI is
// still alive for a moment, and "delete the chat I just made with the wrong provider"
// is an ordinary thing to do seconds after spawning. If that CLI's first session_start
// lands in the window, an unguarded reducer BINDS it: the conversation projection then
// writes a (deletedChatID, sessionID) row AFTER the purge already dropped that chat's
// history. That row is a live trap — a later /resume of the id resolves to a chat that
// does not exist.
//
// The announcement must be DROPPED, and the CLI killed again: it has nowhere to write.
func TestRegression_SessionStartAfterItsChatWasDeleted_IsDroppedNotRecorded(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	termSess := f.runner(t, runnerID).TerminalSession

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	// The CLI is still alive (SIGTERM has not landed yet) and announces its conversation.
	f.announce(t, runnerID, "s-late")

	_, err := f.runners.ChatForSession(f.ctx, "ws1", "s-late")
	assert.ErrorIs(t, err, agentrunner.ErrNotFound,
		"a conversation must never be recorded against a chat that has been deleted")

	assert.Contains(t, f.term.terminateRequestIDs(), termSess,
		"a CLI with nowhere left to write must not be left running")
}

// TestRegression_ResumeIntoAPurgedChat_DoesNotStrandTheRunner
//
// No race needed for this one. Dropping a chat's conversation history is best-effort in
// PurgeChat (a failure is logged and the delete still succeeds), and deleting a Crowbar
// chat deliberately does NOT delete the vendor's own session file — so that conversation
// is still sitting in claude's /resume picker. Pick it, and ChatForSession resolves to
// the purged chat: the runner would be Moved onto a chat that does not exist, and every
// turn it produced from then on would be silently dropped forever. An invisible CLI.
//
// The move must be refused — it is not a move Crowbar can honour — and the CLI retired.
//
// Note what "refused" does NOT mean: it does not mean the runner stays on chat B. The CLI
// really has left sB; that is a fait accompli we cannot undo (spec §3), so recording it as
// still on B would be a lie, and every turn it took in sA would be filed into B's ledger.
// It has followed a conversation somewhere Crowbar cannot follow it, so it is placed
// NOWHERE and killed. Chat B goes dormant — intact, with its history, and resumable.
func TestRegression_ResumeIntoAPurgedChat_DoesNotStrandTheRunner(t *testing.T) {
	f, _, rs := newFaultFixture(t)

	chatA, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	turn(t, f, r1, "claude", "content, so sA is a real conversation on disk")

	// The history drop fails: sA keeps resolving to chatA, which is about to be erased.
	rs.failForgetChat = errors.New("boom: the history drop failed")
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatA))
	require.Equal(t, chatA, f.chatForSession(t, "sA"),
		"precondition: the purged chat's conversation survived the failed drop")

	chatB, r2 := f.spawn(t, "codex")
	f.announce(t, r2, "sB")
	r2term := f.runner(t, r2).TerminalSession

	// The user picks sA out of the CLI's own picker.
	f.announce(t, r2, "sA")

	moved := f.runner(t, r2)
	assert.NotEqual(t, chatA, moved.CurrentChatID,
		"a runner must never be moved onto a chat that does not exist")
	assert.Empty(t, moved.CurrentChatID,
		"it followed a conversation into nowhere: it is placed nowhere")
	assert.Contains(t, f.term.terminateRequestIDs(), r2term,
		"and it must not be left running")

	// Its turns go nowhere — in particular, not into the chat it left.
	require.NoError(t, f.usecase.IngestHook(f.ctx, r2, "codex", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "said inside the purged conversation"})))
	f.wait()
	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatB)
	require.NoError(t, err)
	assert.NotContains(t, handoff, "said inside the purged conversation")

	// And chat B is intact: dormant, with its history, and resumable.
	_, err = f.chats.GetChat(f.ctx, chatB)
	require.NoError(t, err)
	last, err := f.runners.LastConversation(f.ctx, chatB)
	require.NoError(t, err)
	assert.Equal(t, "sB", last.SessionID)
}

// ---------------------------------------------------------------------------
// Displacement is a placement FACT, not an inference from timestamps.
// ---------------------------------------------------------------------------

// TestRegression_LateSessionAnnouncement_DoesNotStealTheChatFromTheIncomingCLI
//
// The window a newest-arrival ORDERING cannot close. The outgoing CLI has not announced
// a conversation yet (claude takes ~1s; the user can hit Switch Provider well inside
// that). The incoming CLI starts at t2 and is the rightful holder of the chat. Then the
// outgoing CLI's session_start finally lands at t3 — and t3 > t2, so any "whoever
// arrived last" ordering hands the chat back to the DYING runner, and the pane attaches
// to a corpse. That is the very failure the ordering existed to prevent.
//
// Displacement fixes it at the source: the CLI we are killing is taken OFF the chat when
// we kill it, so its late announcement resolves to no chat at all and is dropped.
func TestRegression_LateSessionAnnouncement_DoesNotStealTheChatFromTheIncomingCLI(t *testing.T) {
	f := newFixture(t)

	chatID, outgoing := f.spawn(t, "claude") // t0: spawned, and it announces NOTHING yet

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex") // t2
	require.NoError(t, err)
	f.wait()

	// t3: the outgoing CLI, still dying, finally reports the conversation it came up in.
	f.announce(t, outgoing, "s-late")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, incoming, live.ID, "the chat belongs to the incoming CLI, never the corpse")
	assert.Equal(t, f.runner(t, incoming).TerminalSession, live.TerminalSession,
		"the pane must attach to the incoming CLI's PTY")

	assert.Len(t, f.placedRunnersFor(t, chatID), 1,
		"I2: at most one runner is pointed at a chat, at every instant")
}

// TestRegression_UnkillableEvictee_CannotWriteIntoTheChatItLost
//
// An eviction whose kill FAILS leaves the evictee alive — and, if all we did was record
// the mover's Move, the evictee would still be POINTED at the chat the mover now owns.
// Its turn hooks would then append into that chat's ledger and toggle its Working state:
// two CLIs writing one conversation, which is the corruption invariant I3 exists to
// prevent.
//
// Taking it off the chat is a fact Crowbar owns (placement), and it says nothing about
// whether the process is alive — so it is recorded even when the kill fails.
func TestRegression_UnkillableEvictee_CannotWriteIntoTheChatItLost(t *testing.T) {
	f := newFixture(t)

	_, mover := f.spawn(t, "claude")
	f.announce(t, mover, "sA")
	chatB, incumbent := f.spawn(t, "codex")
	f.announce(t, incumbent, "sB")

	f.term.terminateErr = errors.New("boom: the incumbent refuses to die")

	f.announce(t, mover, "sB") // the mover takes sB; the incumbent is evicted but survives

	live, err := f.liveRunnerFor(t, chatB)
	require.NoError(t, err)
	require.Equal(t, mover, live.ID, "precondition: the mover holds chat B")

	// The unkillable evictee keeps working and finishes a turn.
	require.NoError(t, f.usecase.IngestHook(f.ctx, incumbent, "codex", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "the evictee is still talking"})))
	f.wait()

	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatB)
	require.NoError(t, err)
	assert.NotContains(t, handoff, "the evictee is still talking",
		"an evicted CLI must never write into the chat it lost")
	assert.False(t, f.chat(t, chatB).Working,
		"nor toggle its turn state")

	assert.Len(t, f.placedRunnersFor(t, chatB), 1,
		"I3/I2: the evictee is no longer placed anywhere, even though it is still alive")
}

// TestRegression_ConcurrentSwitches_LeaveExactlyOneCLIOnTheChat
//
// Displacement makes the READ side honest. Only serialisation fixes the WRITE side: two
// concurrent SwitchProvider calls both read the same live runner, both kill it, and both
// spawn — leaving two CLIs pointed at one chat, which asynx OCC cannot prevent because
// the two spawns create DIFFERENT aggregates.
//
// The per-chat gate is not the old OpenSegment.Validate guard coming back: it rejects
// nothing, and it never runs on the hook path (a hook must never block or fail). It just
// serialises process creation on a chat, so the second switch sees the first one's runner
// and quits it, exactly as a sequential pair of clicks would.
func TestRegression_ConcurrentSwitches_LeaveExactlyOneCLIOnTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	var g errgroup.Group
	for _, target := range []string{"codex", "claude"} {
		g.Go(func() error {
			_, err := f.usecase.SwitchProvider(f.ctx, chatID, target)
			return err
		})
	}
	require.NoError(t, g.Wait())
	f.wait()

	placed := f.placedRunnersFor(t, chatID)
	assert.Len(t, placed, 1, "two concurrent switches must not leave two CLIs on one chat")

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, placed[0].ID, live.ID)
}

// ---------------------------------------------------------------------------
// Placement onto a chat evicts whoever else is placed there.
// ---------------------------------------------------------------------------

// TestRegression_ResumeIntoAChatHeldByAnotherCLI_EvictsTheChatIncumbent
//
// The reported bug with ONE variable changed, and it needs no race at all.
//
// Eviction used to be keyed only on who holds the CONVERSATION. Nobody looked at who is
// placed on the destination CHAT. So: chat B is being worked by codex (r3), which has not
// announced its conversation yet — or has announced a different one. B's older claude
// conversation sB is still in Crowbar's history AND still in claude's own /resume picker,
// because Crowbar deliberately never deletes a vendor's session file. The user picks sB
// from inside chat A's CLI. Nobody "holds" sB, so nothing is evicted — and chat B ends up
// with TWO live CLIs on it, indefinitely, both appending to its ledger, one of them
// invisible.
//
// Placement onto a chat must evict whoever else is placed there. That is what makes I2 an
// invariant rather than a coincidence.
func TestRegression_ResumeIntoAChatHeldByAnotherCLI_EvictsTheChatIncumbent(t *testing.T) {
	f := newFixture(t)

	// Chat B: claude ran and spoke (so sB is a real, resumable conversation), then the
	// user switched B to codex. Codex (r3) is now working in B and has announced nothing.
	chatB, claudeB := f.spawn(t, "claude")
	f.announce(t, claudeB, "sB")
	turn(t, f, claudeB, "claude", "what claude said in B")
	r3, err := f.usecase.SwitchProvider(f.ctx, chatB, "codex")
	require.NoError(t, err)
	f.wait()
	r3term := f.runner(t, r3).TerminalSession

	require.Equal(t, r3, mustLive(t, f, chatB).ID, "precondition: codex holds chat B")

	// Chat A, elsewhere. Inside its CLI the user /resumes B's old claude conversation.
	_, r1 := f.spawn(t, "claude")
	f.announce(t, r1, "sA")
	f.announce(t, r1, "sB")

	assert.Len(t, f.placedRunnersFor(t, chatB), 1,
		"I2: a chat is held by ONE CLI — the one that just took it, never two")

	live := mustLive(t, f, chatB)
	assert.Equal(t, r1, live.ID, "the mover holds chat B")

	assert.Contains(t, f.term.terminateRequestIDs(), r3term,
		"the CLI that WAS on chat B must be evicted, not left running invisibly")

	// And the evictee can no longer write into the chat it lost.
	require.NoError(t, f.usecase.IngestHook(f.ctx, r3, "codex", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "codex is still talking in B"})))
	f.wait()
	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatB)
	require.NoError(t, err)
	assert.NotContains(t, handoff, "codex is still talking in B")
}

// ---------------------------------------------------------------------------
// Displacement must not orphan the turn the displaced CLI left open.
// ---------------------------------------------------------------------------

// TestRegression_AbortedSwitchMidTurn_DoesNotLeaveTheChatSpinningForever
//
// reconcileRunnerExit closes a turn a dying CLI left open by looking at
// runner.CurrentChatID — which Displace erases. So a switch that ABORTS after the outgoing
// CLI has been quit (an unknown target provider is the tested, reachable case) used to
// leave the chat with no runner and Working=true FOREVER: the chat row spins, and the
// workspace's whole overlay spins with it, until the user resumes that chat and completes
// another turn.
//
// Closing the turn at displacement time asserts nothing about liveness: once displaced, no
// hook from that runner can ever reach the chat again, so "nobody is working on this chat"
// is simply the last true thing we can say about it.
//
// HOW A SWITCH STILL MEETS A MID-TURN CLI, now that it WAITS for an in-flight turn before
// quitting anything (awaitTurnComplete): the user types again while the switch is killing
// it. The barrier is crossed, the turn is genuinely over — and then a new prompt lands in
// the window between the terminate and the displace. That race is exactly what this drives,
// by delivering the prompt hook from INSIDE TerminateGraceful, so the interleaving is
// pinned rather than hoped for.
func TestRegression_InvalidSwitchTarget_PreservesTheOutgoingRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	// Descriptor resolution is deterministic target planning and therefore happens
	// before teardown. An invalid target must not turn a healthy chat dormant.
	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "not-a-real-provider")
	require.Error(t, err)
	f.wait()

	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID)
	assert.Empty(t, f.term.terminatedIDs())
}

// TestSwitchProvider_MidTurn_ClosesTheOutgoingTurn is the same rule on the SUCCESS path:
// the outgoing CLI is displaced with a turn open (the prompt that landed while it was being
// killed — see the regression above), so that turn is over: nobody will ever send its
// turn_stop. The incoming CLI's own turns are unaffected.
func TestSwitchProvider_MidTurn_ClosesTheOutgoingTurn(t *testing.T) {
	f := newFixture(t)

	chatID, outgoing := f.spawn(t, "claude")
	hookDone := make(chan error, 1)
	f.term.duringTerminate = func(string) {
		go func() {
			hookDone <- f.usecase.IngestHook(f.ctx, outgoing, "claude", "user_prompt",
				mustJSON(t, map[string]any{"prompt": "working"}))
		}()
	}

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	require.NoError(t, <-hookDone)
	f.wait()

	assert.False(t, f.chat(t, chatID).Working,
		"the killed CLI's turn is over: it will never send a turn_stop")

	// The incoming CLI can still open its own turn, and the outgoing one's belated PTY
	// death must not close it.
	f.term.duringTerminate = nil
	require.NoError(t, f.usecase.IngestHook(f.ctx, incoming, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "the new CLI is working"})))
	require.True(t, f.chat(t, chatID).Working)

	f.term.exit(t, "term-1")
	f.wait()
	assert.True(t, f.chat(t, chatID).Working,
		"the outgoing runner's exit must not close the incoming runner's turn")
}

// ---------------------------------------------------------------------------
// "" means NOWHERE, everywhere.
// ---------------------------------------------------------------------------

// TestRegression_HookFromADisplacedRunner_NeverTouchesTheChatModel
//
// A displaced-but-still-dying CLI is the NORMAL state of every switched-out, evicted and
// purged runner for as long as SIGTERM takes — and its CurrentChatID is "". An unguarded
// handleTurn hands that "" straight to GetChat, whose lazy self-heal REPLAYS THE ENTIRE
// agentchat EVENT LOG on a miss. So every hook from a dying CLI would replay the whole log.
//
// The fault-injected GetChat is the probe: if the chat model is touched at all, the hook
// fails. It must not be touched — "" is nowhere, and nowhere is not looked up.
func TestRegression_HookFromADisplacedRunner_NeverTouchesTheChatModel(t *testing.T) {
	f, cs, _ := newFaultFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID)) // displaces + kills; the CLI still dies slowly
	f.wait()
	require.Empty(t, f.runner(t, runnerID).CurrentChatID, "precondition: the runner is placed nowhere")

	cs.failGetChat = errors.New("boom: the chat model must not be consulted for nowhere")

	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "a dying CLI's last words"})),
		"a hook from a runner that is placed nowhere must resolve to nowhere, without a lookup")
}

// TestRegression_ExitOfADisplacedRunner_AsksNeitherAggregateAboutNowhere
//
// closeAbandonedTurn's `chatID == ""` check is now the ONLY thing standing between a
// displaced runner and both aggregates, and this pins it.
//
// It did not used to be alone. The function read the chat through GetChat, whose store
// has its OWN "" guard (agentchat store.GetChat refuses an empty id rather than missing
// and replaying the entire event log to heal it). Closing the read-then-act race deleted
// that call, so the backstop went with it — and neither remaining call catches "" by
// itself: LiveRunnerForChat("") returns ErrNotFound, which is exactly the "nobody is on
// this chat" answer that FALLS THROUGH to the abandon, and AbandonTurn("") would then
// hand an empty aggregate id to asynx.
//
// This is the ordinary end of every switched-out, evicted and purged CLI: displaced
// first, dying slowly, and reaching reconcileRunnerExit later with CurrentChatID = "".
// The recorders are the probe — an id reaching either store at all is the failure, and
// nothing else can observe it, because closeAbandonedTurn is best-effort and swallows
// whatever either store returns.
func TestRegression_ExitOfADisplacedRunner_AsksNeitherAggregateAboutNowhere(t *testing.T) {
	f, cs, rs := newFaultFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	term := f.runner(t, runnerID).TerminalSession
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID)) // displaces + kills; the CLI still dies slowly
	f.wait()
	require.Empty(t, f.runner(t, runnerID).CurrentChatID, "precondition: the runner is placed nowhere")

	// The purge's own teardown legitimately consulted both stores about a REAL chat id.
	// Only what the exit does is under test.
	cs.forget()
	rs.forget()

	// And now the SIGTERM'd CLI finally falls over.
	f.term.exit(t, term)
	f.wait()

	assert.Empty(t, rs.liveRunnerForChatIDs(),
		"nowhere is not a chat: the live-runner query must not be asked about it — its ErrNotFound "+
			"reads as 'nobody is on this chat' and would fall straight through to the abandon")
	assert.Empty(t, cs.abandonTurnIDs(),
		"nowhere is not a chat: no turn may be abandoned on an empty aggregate id")
}

// mustLive reads the chat's live runner, failing the test if the chat is dormant.
func mustLive(t *testing.T, f testFixture, chatID string) engineagents.Runner {
	t.Helper()
	r, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	return r
}

// ---------------------------------------------------------------------------
// The rule is general: EVERY placement onto a chat evicts whoever else is there.
// ---------------------------------------------------------------------------

// TestRegression_HookMovesOntoAChatMidSpawn_TheSpawnEvictsIt
//
// The rule "placement onto a chat evicts whoever else is placed there" was applied to
// Move and NOT to Start — and the window that opens is not a hairline, it spans an entire
// process fork:
//
//  1. SwitchProvider holds chat B's gate and quits + DISPLACES the outgoing CLI. Chat B now
//     has ZERO placed runners.
//  2. A hook — never gated, and never may be — moves another live CLI (r1) onto B. Nobody
//     is placed on B and nobody holds that conversation, so nothing is evicted. The Move
//     commits.
//  3. The switch resolves its descriptor, renders its tmp dir, FORKS A PROCESS, and Starts
//     the incoming runner onto B.
//
// Chat B ends up holding r1 AND the incoming CLI, both live, both placed, indefinitely —
// and r1 is invisible, because LiveRunnerForChat hands out the newest arrival while r1 goes
// on appending to B's ledger. The invisible-agent state, restored by the fix that was meant
// to end it.
//
// duringFork drives step 2 INSIDE CreateCommand, so the interleaving is exact rather than
// hoped for: no sleeps, no goroutine racing.
func TestRegression_HookMovesOntoAChatMidSpawn_TheSpawnEvictsIt(t *testing.T) {
	f := newFixture(t)

	// Chat B: claude spoke here, so sB is a real conversation — known to Crowbar, and still
	// in claude's own /resume picker.
	chatB, claudeB := f.spawn(t, "claude")
	f.announce(t, claudeB, "sB")
	turn(t, f, claudeB, "claude", "what claude said in B")

	// Another live CLI, elsewhere.
	_, r1 := f.spawn(t, "codex")
	f.announce(t, r1, "sA")

	// While the switch is forking the incoming CLI, r1's user picks sB out of the picker.
	var once sync.Once
	f.term.duringFork = func() {
		once.Do(func() {
			require.NoError(t, f.usecase.IngestHook(f.ctx, r1, "codex", "session_start",
				mustJSON(t, map[string]any{"session_id": "sB"})))
		})
	}

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatB, "codex")
	require.NoError(t, err)
	f.wait()

	assert.Len(t, f.placedRunnersFor(t, chatB), 1,
		"I2: a chat is held by ONE CLI — a spawn must evict whoever moved in while it forked")

	live := mustLive(t, f, chatB)
	assert.Equal(t, incoming, live.ID, "the chat belongs to the CLI that was spawned onto it")

	assert.Contains(t, f.term.terminateRequestIDs(), f.runner(t, r1).TerminalSession,
		"the CLI that moved in mid-spawn must be evicted, not left running invisibly")

	// And it can no longer write into the chat it was evicted from.
	require.NoError(t, f.usecase.IngestHook(f.ctx, r1, "codex", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "the invisible agent speaks"})))
	f.wait()
	handoff, err := f.usecase.AssembleHandoff(f.ctx, chatB)
	require.NoError(t, err)
	assert.NotContains(t, handoff, "the invisible agent speaks")
}

// TestRegression_EvictionHealsAnAlreadyBrokenInvariant: incumbentsOf used to read ONE row,
// so if two runners were somehow already placed on a chat it would evict one and leave the
// other — healing nothing. "Everyone else placed here is retired" must be literally what
// the code does, or I2 is a coincidence rather than an invariant.
func TestRegression_EvictionHealsAnAlreadyBrokenInvariant(t *testing.T) {
	f := newFixture(t)

	chatB, claudeB := f.spawn(t, "claude")
	f.announce(t, claudeB, "sB")
	turn(t, f, claudeB, "claude", "what claude said in B")

	// Two other live CLIs, both moved onto chat B by hooks landing while it was vacant (the
	// mid-spawn window, twice over — this is the already-broken state).
	_, r1 := f.spawn(t, "codex")
	f.announce(t, r1, "s1")
	_, r2 := f.spawn(t, "codex")
	f.announce(t, r2, "s2")
	require.NoError(t, f.runnersMove(t, r1, chatB, "s1"))
	require.NoError(t, f.runnersMove(t, r2, chatB, "s2"))
	require.Len(t, f.placedRunnersFor(t, chatB), 3, "precondition: the invariant is already broken")

	// A third CLI resumes B's conversation. Everyone else on B must go.
	_, mover := f.spawn(t, "claude")
	f.announce(t, mover, "sMover")
	f.announce(t, mover, "sB")

	assert.Len(t, f.placedRunnersFor(t, chatB), 1,
		"an eviction must retire EVERY other runner placed on the chat, not just one")
	assert.Equal(t, mover, mustLive(t, f, chatB).ID)
}

// TestRegression_FirstAnnouncementOfAHeldConversation_EvictsTheHolder
//
// MoveBind evicts nobody, and its I3 safety used to be an ARGUMENT: a CLI's first
// conversation is one Crowbar itself chose (a fresh spawn, or a resume taken under the
// chat's gate), so it cannot already be held.
//
// That argument is breakable FROM A CONFIG FILE. ResolveDescriptor merges on-disk overrides
// from crowbar home, and spawn.args is user-configurable: drop in an override adding
// `--continue`, or adopt any provider that auto-restores its last session, and a freshly
// spawned CLI announces an id CROWBAR NEVER CHOSE — possibly one another live runner is
// holding. Decide returns MoveBind (a first announcement wins over "known"), nobody is
// evicted, and two CLIs write one provider session file: I3 violated, the PROVIDER'S OWN
// DATA corrupted, with no Go error, no failing test and no log line.
//
// Go must never depend on what the YAML says for its invariants. So the bind evicts the
// holder too — a no-op on every legitimate path, and a guard on this one.
func TestRegression_FirstAnnouncementOfAHeldConversation_EvictsTheHolder(t *testing.T) {
	f := newFixture(t)

	chatA, holder := f.spawn(t, "claude")
	f.announce(t, holder, "sHeld")
	holderTerm := f.runner(t, holder).TerminalSession

	// A second CLI comes up and announces — as its FIRST conversation — the id the first one
	// is live on. (A `--continue`-style descriptor override is all it takes.)
	_, newcomer := f.spawn(t, "claude")
	f.announce(t, newcomer, "sHeld")

	live, err := f.runners.LiveRunnerForSession(f.ctx, "ws1", "sHeld")
	require.NoError(t, err)
	assert.Equal(t, newcomer, live.ID, "the conversation is held by the CLI that announced it last")

	assert.Contains(t, f.term.terminateRequestIDs(), holderTerm,
		"I3: two CLIs must never hold one provider session id — the incumbent is evicted")

	assert.Empty(t, f.placedRunnersFor(t, chatA),
		"the evicted holder is taken off its chat like any other evictee")
}

// TestRegression_TwoPlacementsRacingOneChat_LeaveExactlyOneSurvivor
//
// retireOthersOn told each placement to retire everyone else on the chat. If a hook's Move
// and a spawn's Start commit such that BOTH plural reads happen after BOTH writes, they
// retire each other: the spawn kills the mover, the mover kills the spawn, and the chat is
// left with NOBODY — while SwitchProvider cheerfully returns the id of a runner it has just
// SIGTERM'd, so the pane attaches to a dying PTY.
//
// The list is ordered newest-arrival-first, so the rule is made total by making it
// ASYMMETRIC: retire only the arrivals strictly OLDER than you, and nobody at all if you are
// not on the list (someone else's placement already took you off). Exactly one runner — the
// newest — retires anybody, and it is the same one the serving read hands out. The retire
// rule and the read model agree by construction.
//
// The interleaving is forced through channels inside the committed commands, never timed.
func TestRegression_TwoPlacementsRacingOneChat_LeaveExactlyOneSurvivor(t *testing.T) {
	f, _, rs := newFaultFixture(t)

	chatB, claudeB := f.spawn(t, "claude")
	f.announce(t, claudeB, "sB")
	turn(t, f, claudeB, "claude", "what claude said in B")

	_, r1 := f.spawn(t, "codex")
	f.announce(t, r1, "sA")

	moved := make(chan struct{})
	started := make(chan struct{})
	var hook sync.WaitGroup

	// The hook's Move commits, then it WAITS for the spawn's Start to commit before it runs
	// its own "retire everyone else" read — so both reads see both runners.
	rs.afterMove = func() {
		close(moved)
		<-started
	}
	rs.afterStart = func() { close(started) }

	// The hook fires while the switch is forking its process — after the outgoing CLI has
	// been displaced (the chat is momentarily empty) and before the incoming one is Started.
	f.term.duringFork = func() {
		hook.Add(1)
		go func() {
			defer hook.Done()
			require.NoError(t, f.usecase.IngestHook(f.ctx, r1, "codex", "session_start",
				mustJSON(t, map[string]any{"session_id": "sB"})))
		}()
		<-moved // the Move has committed; let the spawn proceed to Start
	}

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatB, "codex")
	require.NoError(t, err)
	hook.Wait()
	f.wait()

	placed := f.placedRunnersFor(t, chatB)
	require.Len(t, placed, 1, "two placements racing one chat must not retire EACH OTHER")
	assert.Equal(t, incoming, placed[0].ID,
		"the survivor is the newest arrival — the same runner the serving read hands out")

	live := mustLive(t, f, chatB)
	assert.Equal(t, incoming, live.ID)
	assert.NotContains(t, f.term.terminatedIDs(), live.TerminalSession,
		"SwitchProvider must never return a runner it has itself killed")
}
