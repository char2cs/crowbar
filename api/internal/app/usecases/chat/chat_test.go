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
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
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
// delete cascade kills its PROCESS and returns once that exit is recorded, never by
// forgetting a runner whose CLI would still be running, invisible.
func TestRegression_DeleteChat_KillsItsRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	r := f.runner(t, runnerID)
	require.NotEmpty(t, r.TerminalSession)

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	assert.Contains(t, f.term.terminatedIDs(), r.TerminalSession,
		"the chat's CLI must be terminated, not orphaned")
	assert.False(t, f.isLive(runnerID), "no runner may point at a hard-deleted chat")
	_, err := f.liveRunnerFor(t, chatID)
	assert.ErrorIs(t, err, agentusecase.ErrNoLiveRunner)
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
	require.True(t, f.busy(t, chatID), "precondition: the chat is mid-turn when it is deleted")

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

// TestPurgeChat_DropsConversationHistory: a session left owned by a hard-deleted chat
// would make every later announcement of it a conflict with a chat that is gone.
func TestPurgeChat_DropsConversationHistory(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "codex")
	f.announce(t, runnerID, "sid-1")
	require.Equal(t, chatID, f.chatForSession(t, "codex", "sid-1"), "precondition: the conversation is known")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	f.wait()

	assert.Empty(t, f.chatForSession(t, "codex", "sid-1"),
		"the deleted chat's sessions must not keep resolving to it")
	assert.Empty(t, f.conversation(t, chatID, "codex").SessionID)
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
// PLAINTEXT on-disk footprint (its prompt-delivery journal), not only Forget the
// aggregate. The reap targets worktreepath.LedgerChatsDir — keyed by the chat's id
// alone, not a workspace lookup (spec §1.5) — which is where SubmitPrompt et al.
// actually write it (see TestSubmitPrompt_RejectsNULBeforeJournalOrTUITeardown).
//
// It does NOT remove the runner's tmp dir, which is not the chat's to remove: that dir is
// the config of a PROCESS that is still alive (we have asked it to quit, and a SIGTERM is
// not synchronous), and it is keyed by the runner rather than the chat precisely so that it
// can be reaped when the process actually dies — or, if the daemon dies first, at the next
// boot (worktreepath.RunnerDir).
func TestPurgeChat_ReapsChatDirOnDisk(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	chatDir := filepath.Join(worktreepath.LedgerChatsDir(f.ws.home), chatID)
	require.NoError(t, os.MkdirAll(chatDir, 0o700))
	turn(t, f, runnerID, "claude", "a turn, so the chat has a conversation to purge")

	before, err := f.activity.Turns(f.ctx, chatID, 0, 0, 0)
	require.NoError(t, err)
	require.NotEmpty(t, before, "precondition: the chat has a recorded conversation")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	after, err := f.ledger.Read(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, after, "purge must drop the chat's conversation record")

	_, err = os.Stat(chatDir)
	assert.True(t, os.IsNotExist(err), "purge must reap the chat's on-disk dir")
}

// TestPurgeChat_ReapFailure_StillPurges: the on-disk reap is best-effort — even if
// crowbar home cannot be resolved, the aggregate is still Forgotten and PurgeChat
// returns nil rather than failing a delete the user asked for.
//
// It faults f.homeErr, not f.ws.err: the reap no longer resolves a workspace at all
// (worktreepath.LedgerChatsDir needs only crowbar home), which is the whole point —
// a workspace-less chat's reap must not depend on a workspace lookup either.
func TestPurgeChat_ReapFailure_StillPurges(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	*f.homeErr = errors.New("boom: crowbar home lookup for reap")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID), "a reap-path failure must not abort the purge")

	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound, "the aggregate is still Forgotten")
}

// TestPurgeChat_UnknownChat_ReturnsWrappedError: PurgeChat on an id with no chat wraps
// the lookup failure rather than panicking or silently no-oping.
func TestPurgeChat_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.PurgeChat(f.ctx, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "purge chat: get")
}

// TestPurgeChat_UnknownChat_AlsoAnswersApperrNotFound is the regression for a
// live bug: tree.Agent's contract (Usecase is one of its implementations) is
// apperr.ErrNotFound for "nothing to purge" — that's what purgeAll's cascade
// tolerance checks for — but this method used to hand back only the
// conversations package's own agentchat.ErrNotFound, wrapped. purgeAll's
// errors.Is(err, apperr.ErrNotFound) never matched a real not-found, so
// deleting a parent with one never-minted descendant, or a chat whose
// aggregate never got created, failed the whole delete with the raw
// "agentchat: not found" chain surfaced straight to the user.
func TestPurgeChat_UnknownChat_AlsoAnswersApperrNotFound(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.PurgeChat(f.ctx, "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, apperr.ErrNotFound)
	assert.ErrorIs(t, err, agentchat.ErrNotFound, "the original cause stays inspectable too")
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
		mustJSON(t, map[string]any{"last_assistant_message": "done thing"})))

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

// injectedContext is the context document spawn carried: on its fresh launch's
// system prompt, or in the system reminder a resume is handed.
func injectedContext(
	t *testing.T,
	f testFixture,
	spawn int,
) string {
	t.Helper()
	argv := f.term.calls[spawn].argv
	if at := indexOf(argv, "--append-system-prompt"); at >= 0 && at+1 < len(argv) {
		return argv[at+1]
	}
	for _, a := range argv {
		if strings.HasPrefix(a, "<system-reminder>") {
			return a
		}
	}
	require.Fail(t, "no context channel", "spawn %d carried no context: %v", spawn, argv)
	return ""
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

// file puts a folder row in the tree: a domain.Folder+domain.Node pair,
// exactly like the tree usecase's own Create does (2026-09-08
// sidebar-placement-unification Task 8 — a folder is never a Chat row any
// more, home-scoped or repo-scoped alike). The lineage walk reads it back
// through f.usecase's own wrapped ChatLineage (harness_test.go's
// NewHomeCorrectedTreeChats wiring), the same door production reads it
// through.
func file(
	t *testing.T,
	f testFixture,
	id string,
	parentID string,
) {
	t.Helper()
	f.folders.Saved = append(f.folders.Saved, domain.Folder{ID: id, Name: id})
	_, err := f.nodes.Create(f.ctx, id, domain.NodeKindFolder, parentID, 0)
	require.NoError(t, err)
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

// spawnPlaced mints a claude chat, places it under parentID ("" leaves it
// unplaced) and only then spawns it: a chat is told what it reads at its first launch.
func spawnPlaced(t *testing.T, f testFixture, parentID string) string {
	t.Helper()
	chatID, err := f.usecase.MintChat(f.ctx, "ws1", "claude", "")
	require.NoError(t, err)
	f.wait()
	if parentID != "" {
		thread(t, f, chatID, parentID)
	}
	_, err = f.usecase.ResumeChat(f.ctx, chatID)
	require.NoError(t, err)
	f.wait()
	return chatID
}

// A thread is SPAWNED knowing what it reads. Not handed the turns — handed the
// ids and told to go and get them, which is what makes the relationship live:
// the parent's later turns are in the answer too.
func TestSpawn_AThreadIsPointedAtTheChatItHangsOff(t *testing.T) {
	f := newFixture(t)

	parentID, parentRunner := f.spawn(t, "claude")
	turn(t, f, parentRunner, "claude", "the parent worked out the plan here")
	spawnPlaced(t, f, parentID)

	got := injectedContext(t, f, 1)
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
	file(t, f, "outer", parentID)
	file(t, f, "inner", "outer")
	spawnPlaced(t, f, parentID)
	spawnPlaced(t, f, "inner")

	assert.Contains(t, injectedContext(t, f, 1), lineageBlock(parentID))
	assert.Contains(t, injectedContext(t, f, 2), lineageBlock(parentID),
		"filing a thread away is organisation; it must not change one word of what it reads")
}

// Nearest parent first, because that is the order the ids are useful in.
func TestSpawn_ADeepThreadIsToldItsWholeChainNearestFirst(t *testing.T) {
	f := newFixture(t)

	grandparentID, _ := f.spawn(t, "claude")
	parentID, _ := f.spawn(t, "claude")
	thread(t, f, parentID, grandparentID)
	spawnPlaced(t, f, parentID)

	assert.Contains(t, injectedContext(t, f, 2), lineageBlock(parentID, grandparentID))
}

// A chat with no chat ancestors must behave exactly as it does today — not
// "almost", exactly, which is why this compares against a chat that has never
// been near the tree rather than against a substring.
func TestSpawn_AChatWithNoChatAncestorsIsToldNothingExtra(t *testing.T) {
	f := newFixture(t)

	file(t, f, "somewhere", "")
	spawnPlaced(t, f, "")
	spawnPlaced(t, f, "somewhere")

	assert.Equal(t, injectedContext(t, f, 0), injectedContext(t, f, 1),
		"a chat merely filed in a folder inherits nothing and is spawned identically to an unfiled one")
	assert.NotContains(t, injectedContext(t, f, 1), "THREAD CONTEXT")
}

// A lineage that cannot be read fails the spawn. A thread that comes up silently
// believing itself standalone would then do the whole task without the context
// it exists to continue, and nothing anywhere would say so.
func TestSpawn_ALineageThatCannotBeReadFailsTheSpawn(t *testing.T) {
	f, cs := newFaultFixture(t)

	parentID, _ := f.spawn(t, "claude")
	threadID, _ := f.spawn(t, "claude")
	thread(t, f, threadID, parentID)
	cs.failListByWorkspace = errors.New("folder table unreadable")

	_, err := f.usecase.SwitchProvider(f.ctx, threadID, "claude")
	require.ErrorContains(t, err, "folder table unreadable")
	require.Len(t, f.term.calls, 2, "and no CLI was started")
}

// The chat a spawn is MINTING cannot have been placed anywhere yet — the
// aggregate is written after the CLI is live — so that spawn resolves no lineage
// at all rather than failing on a chat that does not exist.
func TestSpawn_MintingAChatResolvesNoLineage(t *testing.T) {
	f, cs := newFaultFixture(t)
	cs.failListByWorkspace = errors.New("folder table unreadable")

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

// The note is tagged as Crowbar's own, never as a provider's: a note wearing a
// provider's name would make a session it never spoke in look resumable.
func TestNoteThreadLineage_IsNotAttributedToAnyProvider(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude")
	require.NoError(t, f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1"}))
	f.wait()

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()
	assert.Equal(t, -1, indexOf(f.term.calls[2].argv, "--resume"), "claude never spoke in sid-claude")
}

// ---------------------------------------------------------------------------
// The two halves of a placed create
// ---------------------------------------------------------------------------

// MintChat writes the chat and starts nothing. The chat that comes back is a
// DORMANT one — the state the panel already models — so the caller is free to
// place it before deciding to start anything on it.
func TestMintChat_CreatesTheChatAndNoRunner(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "ws1", "", "")
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
	f, chatStore := newFaultFixture(t)
	chatStore.failCreate = errors.New("chat store down")

	_, err := f.usecase.MintChat(f.ctx, "ws1", "", "")
	require.ErrorContains(t, err, "chat store down")
}

// The regression this whole path exists for, at the unit level: a chat that is
// ALREADY a thread when its first CLI is started is told so on that first
// session. StartRunner takes the ordinary create=false spawn, so nothing
// special-cases it — it falls out of the chat existing and being placed first.
func TestStartRunner_ATheadPlacedBeforeItsFirstSpawnIsToldItsLineage(t *testing.T) {
	f := newFixture(t)

	parentID, _ := f.spawn(t, "claude")
	threadID, err := f.usecase.MintChat(f.ctx, "ws1", "", "")
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

	chatID, err := f.usecase.MintChat(f.ctx, "ws1", "", "")
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

	chatID, err := f.usecase.MintChat(f.ctx, "ws1", "", "")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1"}))

	turns, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, turns, "a chat with no history has no move to date")
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
	f, chatStore := newFaultFixture(t)

	parentID, _ := f.spawn(t, "claude")
	threadID, err := f.usecase.MintChat(f.ctx, "ws1", "", "")
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

// resumeClaudeWithGap's callers stamp session_id: "sid-claude", the session the
// resumed runner was launched on, on the user_prompt they deliver.
//
// resumeClaudeWithGap drives the one path where Crowbar's own context document comes
// back at it as a user prompt: claude switched away and then BACK. A resumed
// hooks-transport CLI cannot be reached through any config channel (verified against
// 0.139.0), so the gap is delivered as a positional — which IS claude's first user
// message, and which its user-prompt hook duly reports. Returns the new claude runner
// and the exact document it was spawned with.
//
// claude, not codex: codex declares no resume_context_inject at all, so its gap never
// rides argv on any transport — and when its api connection IS live, apiResumes
// (resume_injection.go) additionally withholds the whole resume from the companion
// PTY, which would otherwise answer the pointer as a second, disconnected "codex"
// conversation (confirmed live). claude has no such competing connection: its PTY IS
// the conversation, so this mechanism is its live, correct delivery path.
func resumeClaudeWithGap(t *testing.T, f testFixture) (chatID, claudeRunnerID, injected string) {
	t.Helper()

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude")
	exchange(t, f, claudeRunner, "claude", "claude said this before leaving")

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	// codexTurnStop delivers codex's real api-shaped turn_stop (threadId/
	// turn.items[type=agentMessage].text — see codex.yaml), NOT the flat
	// last_assistant_message shape turn() builds for claude — using turn()
	// here would silently extract an empty message.
	f.announce(t, codexRunner, "sid-codex-away")
	codexTurnStop(t, f, codexRunner, "sid-codex-away", "codex spoke while claude was away")

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
		mustJSON(t, map[string]any{"prompt": injected, "session_id": "sid-claude"})))
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
		mustJSON(t, map[string]any{"prompt": injected, "session_id": "sid-claude"})))
	f.wait()

	assert.True(t, f.busy(t, chatID), "the CLI is answering the gap: the chat must read as working")
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
			mustJSON(t, map[string]any{"prompt": injected, "session_id": "sid-claude"})))
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
	_, found := f.engine.ConsumeInjectedPrefix(claudeRunner, injected)
	require.True(t, found)

	// Re-arm it (the match above consumed it), then kill the PTY.
	f.engine.RecordInjection(claudeRunner, injected)
	f.term.exit(t, f.runner(t, claudeRunner).TerminalSession)
	f.wait()

	_, found = f.engine.ConsumeInjectedPrefix(claudeRunner, injected)
	assert.False(t, found, "a dead runner's injected context must be forgotten")
}

// ─── from selection_test.go ───────────────────────────────────────────

func writeDescriptor(t *testing.T, f testFixture, id, body string) {
	t.Helper()
	dir := filepath.Join(f.ws.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(withLifecycle(body)), 0o600))
}

// withLifecycle completes a test descriptor that declares only the part under
// test with the wired lifecycle every descriptor needs to be enabled at all.
func withLifecycle(body string) string {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil || doc == nil {
		return body // a deliberately broken document stays broken
	}
	events, _ := doc["events"].(map[string]any)
	if events == nil {
		events = map[string]any{}
	}
	for name, spec := range map[string]map[string]any{
		"session_start": {"in": "SessionStart", "map": map[string]any{"session_id": "session_id"}},
		"user_prompt":   {"in": "UserPromptSubmit", "map": map[string]any{"session_id": "session_id", "message": "prompt"}},
		"turn_stop":     {"in": "Stop", "map": map[string]any{"session_id": "session_id", "message": "last_assistant_message"}},
	} {
		if _, ok := events[name]; !ok {
			spec["required"] = []string{"session_id"}
			events[name] = spec
		}
	}
	doc["events"] = events
	if _, ok := doc["hooks_injection"]; !ok {
		doc["hooks_injection"] = []any{map[string]any{"pass_arg": map[string]any{
			"arg": "--hook", "value": "{crowbar_hook} hook any --segment {segid}",
		}}}
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return body
	}
	return string(out)
}

const selectingDescriptorBody = `
id: claude
display_name: Selecting
spawn:
  cmd: claude
  interactive_required: true
  forbid_flags: ["-p", "--print"]
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
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
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
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

// TestSetChatSelection_RecordsChatSwitchInterruptions guards the new
// InterruptModelChanged/InterruptEffortChanged markers this fix adds:
// recorded independently, only when the corresponding value actually
// changes, and never on a no-op re-application of the same selection.
//
// Every read here waits first: Interrupt is on the async Send path, so the
// interruption is durable in the log before its projection folds, and a read
// taken without the drain is a read of the row set one event ago.
func TestSetChatSelection_RecordsChatSwitchInterruptions(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	t.Run("model and effort both change from empty", func(t *testing.T) {
		require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))
		f.wait()

		f.wait()
		ints, err := f.activity.Interruptions(f.ctx, chatID)
		require.NoError(t, err)
		require.Len(t, ints, 2)
		kinds := []string{ints[0].Kind, ints[1].Kind}
		assert.Contains(t, kinds, ledger.MarkerModelChanged)
		assert.Contains(t, kinds, ledger.MarkerEffortChanged)
		for _, i := range ints {
			if i.Kind == ledger.MarkerModelChanged {
				assert.Equal(t, "opus", i.Detail)
			}
			if i.Kind == ledger.MarkerEffortChanged {
				assert.Equal(t, "high", i.Detail)
			}
			assert.NotNil(t, i.ResolvedAt, "Crowbar's own doing: opened and resolved together")
		}
	})

	t.Run("reapplying the same selection records nothing new", func(t *testing.T) {
		require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))
		f.wait()

		f.wait()
		ints, err := f.activity.Interruptions(f.ctx, chatID)
		require.NoError(t, err)
		assert.Len(t, ints, 2, "no new interruption for a value that did not change")
	})

	t.Run("effort-only change records only InterruptEffortChanged", func(t *testing.T) {
		require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "low"))
		f.wait()

		f.wait()
		ints, err := f.activity.Interruptions(f.ctx, chatID)
		require.NoError(t, err)
		require.Len(t, ints, 3)
		assert.Equal(t, ledger.MarkerEffortChanged, ints[2].Kind)
		assert.Equal(t, "low", ints[2].Detail)
	})
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

// TestRegression_SetChatSelection_ConcurrentStandaloneAndStagedNeverLogAStaleChange
// guards the standalone PATCH .../selection route (Usecase.SetChatSelection)
// against racing a SubmitPrompt call that stages its OWN model/effort change
// on the same chat: the standalone route reads the chat's CURRENT selection,
// writes its new one, then records which of model/effort actually differed —
// entirely unguarded by rs.spawns, unlike every other mutating entry point on
// this interface (SwitchProvider, StopChat, SwitchToTerminal, SwitchToNative,
// SubmitPromptWithSwitch). Two concurrent writers can each read a "before"
// snapshot taken before the OTHER's write has landed, so the interruption log
// ends up narrating a transition to a value a concurrent write immediately
// superseded — the chat settles on one caller's pick, but the log can still
// carry a "changed to" entry for the OTHER caller's, never corrected.
func TestRegression_SetChatSelection_ConcurrentStandaloneAndStagedNeverLogAStaleChange(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	start := make(chan struct{})
	done := make(chan error, 2)
	go func() {
		<-start
		done <- f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high")
	}()
	go func() {
		<-start
		_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hi", uuid.NewString(), "", &domain.ChatSelection{Model: "haiku", Effort: "low"})
		done <- err
	}()
	close(start)
	require.NoError(t, <-done)
	require.NoError(t, <-done)
	f.wait()

	final := f.chat(t, chatID)
	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)

	var lastModelDetail string
	found := false
	for _, i := range ints {
		if i.Kind == ledger.MarkerModelChanged {
			lastModelDetail = i.Detail
			found = true
		}
	}
	require.True(t, found, "at least one model-change interruption must be recorded")
	assert.Equal(t, final.Model, lastModelDetail,
		"the most recently recorded model-change entry must describe the model the chat "+
			"actually ended up on (final=%q, logged=%q) — never a value a concurrent write "+
			"immediately superseded", final.Model, lastModelDetail)
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
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
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
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "hello", uuid.NewString(), "", nil)
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

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "with a model", uuid.NewString(), "", nil)
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

// The composer must never PATCH .../selection on its own — a staged pick has
// to travel WITH the send, committed by SubmitPrompt itself, not by a prior
// call. No SetChatSelection call precedes this one.
func TestSubmitPrompt_CommitsAStagedSelectionItselfBeforeSpawning(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "", &domain.ChatSelection{Model: "opus", Effort: "high"})
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

// THE REGRESSION this session fixed. The picker used to call SwitchProvider
// itself, live, the instant a cross-provider row was clicked — before the
// user ever sent anything. Now the picker only stages the pick, and the
// switch happens here, atomically with the send: one path, not two.
func TestSubmitPrompt_StagedProviderSwitchesBeforeDelivering(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "codex", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "codex", live.ProviderID, "the switch actually ran before delivery")

	// SwitchProvider's own distinctive mark — see
	// TestSwitchProvider_RecordsAProviderSwitchedInterruption — is exactly one
	// interruption naming the target. Asserting on it rather than a raw
	// terminal-call count keeps this independent of "codex" delivery's own
	// mechanics (a real switch always fires it exactly once; ordinary prompt
	// delivery, restart-based or not, never does).
	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, ledger.MarkerProviderSwitched, ints[0].Kind)
	assert.Equal(t, "codex", ints[0].Detail)
}

// A resend under the SAME provider the chat is already on must never pay for
// a switch — SwitchProvider kills and respawns the CLI unconditionally
// whenever it runs (see switchProviderLocked), so calling it on every ordinary
// message would tear down and restart the CLI on every send. Compared as a
// DELTA against an ordinary (nothing-staged) submit on the same chat, not a
// raw call count, since this fixture's own delivery may restart the TUI for
// reasons that have nothing to do with provider — the thing under test is
// whether STAGING the current provider costs anything ON TOP of that.
func TestSubmitPrompt_SameProviderNeverTriggersASwitch(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	spawned := f.term.callCount()
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "first", uuid.NewString(), "", nil)
	require.NoError(t, err)
	ordinaryDelta := f.term.callCount() - spawned

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	// A busy chat refuses the next submit — confirm dispatch, then close the
	// turn, exactly as the CLI's own hooks would in production.
	prompt(t, f, live.ID, "claude", "first")
	turn(t, f, live.ID, "claude", "reply")
	afterOrdinary := f.term.callCount()

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "second", uuid.NewString(), "claude", nil)
	require.NoError(t, err)
	stagedSameProviderDelta := f.term.callCount() - afterOrdinary

	assert.Equal(t, ordinaryDelta, stagedSameProviderDelta,
		"staging the provider the chat is already on must cost exactly what an ordinary send costs")

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, ints, "the provider never changed — nothing for SwitchProvider to have recorded")
}

// A staged pick that is not jointly valid (effort the model doesn't declare)
// must refuse the WHOLE send, exactly as a standalone PATCH .../selection
// would — and never deliver the prompt on a selection that failed to commit.
func TestSubmitPrompt_RefusesAndNeverSpawnsOnAnInvalidStagedSelection(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	callsBefore := f.term.callCount()

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "", &domain.ChatSelection{Model: "gpt-5", Effort: ""})

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Equal(t, callsBefore, f.term.callCount(), "an invalid selection must not spawn a replacement runner")
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

// permissionSelectingDescriptorBody extends selectingDescriptorBody with a
// permission_levels block declared restart_tui, matching claude's and codex's own
// real descriptors — needed to exercise the third clause of
// selection.RestartRequired (PermissionLevel), which none of this suite's other
// selecting-descriptor fixtures ever declare.
const permissionSelectingDescriptorBody = selectingDescriptorBody + `
permission_levels:
  strategy: restart_tui
  levels:
    guarded:
      apply:
        - pass_arg: { arg: "--permission-mode", value: "manual" }
    full-auto:
      apply:
        - pass_arg: { arg: "--permission-mode", value: "auto" }
`

// TestRegression_PermissionLevelUnchangedDoesNotForceARestartADeliveryWouldNotHaveDone
// guards RequirePromptRestart's `launched` selection literal, which built its
// Selection from live.LaunchModel/live.LaunchEffort only and left PermissionLevel
// at its zero value. That reads as "changed" against literally any chat's current
// level, because no chat's desired PermissionLevel is ever empty (every chat is
// seeded with a real level at creation — see domain.Chat's own doc comment): the
// zero-value stand-in just as reliably differs from a genuinely UNCHANGED level as
// from a genuinely different one, so a naive "prove a real switch authorises a
// restart" test (below) passes whether or not the field is wired up — it can never
// turn red. Only holding the level in place, as this test does, tells the two
// readings apart.
func TestRegression_PermissionLevelUnchangedDoesNotForceARestartADeliveryWouldNotHaveDone(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", permissionSelectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	require.Equal(t, "guarded", live.LaunchPermissionLevel,
		"precondition: spawned under the fixture's own pinned default")

	shipped, err := f.engine.Get(f.ctx, f.ws.home, "claude")
	require.NoError(t, err)
	descriptor := undeliverableAgent{Agent: shipped}
	require.NotEqual(t, engineagents.DeliveryRestartTUI, descriptor.Capabilities().Delivery,
		"the fixture must not restart for delivery reasons, or this proves nothing")

	err = agentusecase.RequirePromptRestart(f.ctx, f.usecase.RunnerUsecase, chatID, live, descriptor)

	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported,
		"the chat's permission level never changed from what this runner launched under; "+
			"a delivery this daemon has no channel for must stay refused")
}

// TestSubmitPrompt_PermissionLevelSwitchAloneAuthorisesARestartADeliveryWouldNotHaveDone
// is PermissionLevel's own counterpart to
// TestSubmitPrompt_ASelectionSwitchAuthorisesARestartADeliveryWouldNotHaveDone,
// completing that test's Model/Effort coverage with the third field
// selection.RestartRequired compares. Note this one passes with or without
// RequirePromptRestart's launched-selection fix — see the steady-state test above,
// which is the one that actually catches the bug.
func TestSubmitPrompt_PermissionLevelSwitchAloneAuthorisesARestartADeliveryWouldNotHaveDone(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", permissionSelectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	shipped, err := f.engine.Get(f.ctx, f.ws.home, "claude")
	require.NoError(t, err)
	descriptor := undeliverableAgent{Agent: shipped}

	require.NoError(t, f.usecase.SetChatPermissionLevel(f.ctx, chatID, "full-auto"))

	require.NoError(t, agentusecase.RequirePromptRestart(f.ctx, f.usecase.RunnerUsecase, chatID, live, descriptor),
		"a pending permission-level switch obliges a restart on its own")
}

func TestSubmitPrompt_TheRestartResumesTheNativeConversation(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "native-session")
	exchange(t, f, runnerID, "claude", "the conversation exists")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "keep my history", uuid.NewString(), "", nil)

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
	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "under opus", uuid.NewString(), "", nil)
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(f.ctx, first.RunnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "under opus"})))
	turn(t, f, first.RunnerID, "claude", "answered")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "back to default", uuid.NewString(), "", nil)

	require.NoError(t, err)
	call := f.term.calls[f.term.callCount()-1]
	assert.Less(t, indexOf(call.argv, "--model"), 0, "the default carries no model flag")
}

// THE REGRESSION. "" is a REAL pick — "the provider's own default", the same
// distinct fact domain.Chat.Model's own doc records — and the composer stages
// it like any other. Gated on "either half is non-empty" (as this path once
// was) an all-empty pick read as "nothing staged, keep what you have", so a
// chat pinned to opus had NO way back to the default at all: the picker no
// longer PATCHes .../selection on its own, which makes the send the only door.
// The test above proves the same clearing still works through the standalone
// PATCH; this one proves it works through the door the UI actually uses.
func TestRegression_SubmitPrompt_AnExplicitlyEmptyStagedSelectionClearsBackToTheDefault(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))

	_, err := f.usecase.SubmitPrompt(
		f.ctx, chatID, "back to default", uuid.NewString(), "", &domain.ChatSelection{})

	require.NoError(t, err)
	chat := f.chat(t, chatID)
	assert.Empty(t, chat.Model, "an explicitly empty pick clears the sticky model")
	assert.Empty(t, chat.Effort)
	call := f.term.calls[f.term.callCount()-1]
	assert.Less(t, indexOf(call.argv, "--model"), 0, "the default carries no model flag")
	assert.Less(t, indexOf(call.argv, "--effort"), 0, "nor an effort one")
}

// A nil selection is the OTHER half of the same contract: the composer staged
// nothing, so the chat's sticky pair must survive the send untouched. Without
// this, making "" mean "clear it" would quietly turn every caller that passes
// no selection at all into one that wipes the chat's model.
func TestSubmitPrompt_ANilStagedSelectionLeavesTheStickyPairAlone(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", "high"))

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "nothing staged", uuid.NewString(), "", nil)

	require.NoError(t, err)
	chat := f.chat(t, chatID)
	assert.Equal(t, "opus", chat.Model)
	assert.Equal(t, "high", chat.Effort)
}

// Idempotency is the whole point of clientRequestId, and the staged selection
// now rides the SAME body. A retry must replay the original outcome and must
// not narrate the selection change a second time — setChatSelectionLocked
// records a divider only against the value it READ first, so a replay of an
// already-committed pick has nothing left to differ from.
func TestRegression_SubmitPrompt_ReplayingAStagedSelectionStaysAtMostOnce(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, _ := f.spawn(t, "claude")
	requestID := uuid.NewString()
	staged := &domain.ChatSelection{Model: "opus", Effort: "high"}

	first, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID, "", staged)
	require.NoError(t, err)
	spawned := f.term.callCount()

	second, err := f.usecase.SubmitPrompt(f.ctx, chatID, "at most once", requestID, "", staged)

	require.NoError(t, err)
	assert.Equal(t, first, second, "the replay answers with the original delivery identity")
	assert.Equal(t, spawned, f.term.callCount(), "and never forks a second CLI")
	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	changes := 0
	for _, interruption := range ints {
		if interruption.Kind == ledger.MarkerModelChanged ||
			interruption.Kind == ledger.MarkerEffortChanged {
			changes++
		}
	}
	assert.Equal(t, 2, changes, "one model divider and one effort divider, not two of each")
}

func TestSubmitPrompt_NoSwitchUnderARestartingDeliveryIsUnchanged(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "ordinary message", uuid.NewString(), "", nil)

	require.NoError(t, err)
	assert.Equal(t, 2, f.term.callCount(), "the original spawn plus the delivery restart")
}

// TestRegression_UnseededChatPermissionLevelSpawnsUnderGlobalDefaultNotGuarded
// guards a chat that predates PermissionLevel seeding entirely — the
// "carried through the chat-model migration, never had this field" case
// domain.Chat's own doc comment documents as a real, if transient, edge
// case. f.chats.Create bypasses Conversations.MintChat/SeedPermissionLevel
// on purpose, to construct exactly that state: every real chat-creation
// path this package wires up already seeds a real level, so a chat spawned
// through the ordinary helpers can never observe it.
func TestRegression_UnseededChatPermissionLevelSpawnsUnderGlobalDefaultNotGuarded(t *testing.T) {
	f := newFixtureWithPermissionDefault(t, "full-auto")
	writeDescriptor(t, f, "claude", permissionSelectingDescriptorBody)

	chatID := uuid.NewString()
	_, err := f.chats.Create(f.ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: "ws1", Type: domain.ChatTypeChat, Now: time.Now(),
	})
	require.NoError(t, err)
	f.wait()
	chat, err := f.chats.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	require.Empty(t, chat.PermissionLevel, "precondition: an unseeded chat")

	_, err = f.usecase.StartRunner(f.ctx, chatID, "claude")
	require.NoError(t, err)

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "full-auto", live.LaunchPermissionLevel,
		"an unseeded chat must spawn under the GLOBAL default, never silently guarded")
}

// TestRegression_UnseededChatHonoursAnExplicitLevelOnItsNextRestart is the real,
// confirmed-live sequence: a chat that predates seeding (see the test above) has
// its permission level explicitly PUT to full-auto, then its very next prompt —
// which always forces a full CLI restart for a restart_tui provider like claude —
// must launch under that explicit choice, not fall back to guarded.
func TestRegression_UnseededChatHonoursAnExplicitLevelOnItsNextRestart(t *testing.T) {
	f := newFixtureWithPermissionDefault(t, "guarded")
	writeDescriptor(t, f, "claude", permissionSelectingDescriptorBody)

	chatID := uuid.NewString()
	_, err := f.chats.Create(f.ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: "ws1", Type: domain.ChatTypeChat, Now: time.Now(),
	})
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.StartRunner(f.ctx, chatID, "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.SetChatPermissionLevel(f.ctx, chatID, "full-auto"))

	_, err = f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "", nil)
	require.NoError(t, err)

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "full-auto", live.LaunchPermissionLevel,
		"the restart the next prompt forces must launch under the level just explicitly set")
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
	// Resolved synchronously off the embedded model-manifest.json bundle
	// (model.manifest:, not a live probe) — deterministic even offline.
	assert.Equal(t, []string{"fable", "opus", "sonnet", "haiku", "opusplan"}, claude.Models)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, claude.Efforts[""],
		"the '' key is the union of every model's own levels — see modeldiscovery.effortsOf")
	for _, model := range claude.Models {
		assert.NotEmpty(t, claude.Efforts[model], "every selectable model must answer its levels")
	}
	assert.Empty(t, claude.DefaultModel, "claude's manifest states no default, so this is unknown")

	// codex's catalogue is DISCOVERED at runtime (model.discover:), not
	// declared — with no codex binary in this test environment nothing has
	// resolved, so both are empty rather than a stale hand-maintained list.
	// The discovery mapping itself is modeldiscovery's own tests.
	codex := list[byID["codex"]]
	assert.True(t, codex.ModelSelect)
	assert.True(t, codex.EffortSelect)
	assert.Empty(t, codex.Models)
	assert.Empty(t, codex.Efforts)
	assert.Empty(t, codex.DefaultModel,
		"codex's own debug models states no per-model default — unknown, never Models[0]")
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
	chatID, err := f.usecase.MintChat(f.ctx, "ws1", "", "")
	require.NoError(t, err)
	f.wait()

	err = f.usecase.SetChatSelection(f.ctx, chatID, "opus", "")

	require.ErrorIs(t, err, apperr.ErrUnprocessable)

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "", ""))
}

func TestSetChatSelection_SaveFailureSurfaces(t *testing.T) {
	f, cs := newFaultFixture(t)
	chatID, _ := f.spawn(t, "claude")
	cs.failSetSelection = errors.New("boom: save selection")

	err := f.usecase.SetChatSelection(f.ctx, chatID, "opus", "")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "save")
}

func TestSpawn_AnUnreadableSelectionFailsBeforeTheFork(t *testing.T) {
	f, cs := newFaultFixture(t)
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
session: { resume: { arg: "--resume {id}" }, persists: turn_start }
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

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "nowhere to go", uuid.NewString(), "", nil)

	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported)
}

func TestSubmitPrompt_AnUnreadableSelectionRefusesTheDelivery(t *testing.T) {
	f, cs := newFaultFixture(t)
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
// history. That row is a live trap — a later announcement of the id conflicts with a
// chat that does not exist.
//
// The announcement must be REFUSED, and the CLI killed: it has nowhere to write.
func TestRegression_SessionStartAfterItsChatWasDeleted_IsDroppedNotRecorded(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	termSess := f.runner(t, runnerID).TerminalSession

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	// The CLI is still alive (SIGTERM has not landed yet) and announces its conversation.
	lateHook(t, f, runnerID, "", "session_start", map[string]any{"session_id": "s-late"})

	assert.Empty(t, f.chatForSession(t, "claude", "s-late"),
		"a conversation must never be recorded against a chat that has been deleted")

	assert.Contains(t, f.term.terminateRequestIDs(), termSess,
		"a CLI with nowhere left to write must not be left running")
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
// we kill it, so its late announcement moves no chat and is only dead-lettered.
func TestRegression_LateSessionAnnouncement_DoesNotStealTheChatFromTheIncomingCLI(t *testing.T) {
	f := newFixture(t)

	chatID, outgoing := f.spawn(t, "claude") // t0: spawned, and it announces NOTHING yet

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex") // t2
	require.NoError(t, err)
	f.wait()

	// t3: the outgoing CLI, still dying, finally reports the conversation it came up in.
	hook(t, f, outgoing, "", "session_start", map[string]any{"session_id": "s-late"})

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, incoming, live.ID, "the chat belongs to the incoming CLI, never the corpse")
	assert.Equal(t, f.runner(t, incoming).TerminalSession, live.TerminalSession,
		"the pane must attach to the incoming CLI's PTY")

	assert.False(t, f.isLive(outgoing), "the outgoing CLI was off the chat before the switch returned")
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

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	var onChat []string
	for _, r := range agentusecase.AllLiveRunners(f.usecase.own) {
		if r.CurrentChatID == chatID {
			onChat = append(onChat, r.ID)
		}
	}
	assert.Equal(t, []string{live.ID}, onChat, "two concurrent switches must not leave two CLIs on one chat")
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

// The same rule on the success path: the outgoing CLI is displaced with a turn open (a
// prompt landed while it was killed), so that turn is over — nothing will send its
// turn_stop. The incoming CLI's own turns are unaffected.
func TestSwitchProvider_MidTurn_ClosesTheOutgoingTurn(t *testing.T) {
	f := newFixture(t)

	chatID, outgoing := f.spawn(t, "claude")
	var once sync.Once
	hookDone := make(chan error, 1)
	f.term.duringTerminate = func(string) {
		once.Do(func() {
			hookDone <- f.usecase.IngestHook(f.ctx, outgoing, "claude", "user_prompt",
				mustJSON(t, map[string]any{"prompt": "working"}))
		})
	}

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")
	require.NoError(t, err)
	require.NoError(t, <-hookDone)
	f.wait()

	assert.False(t, f.busy(t, chatID),
		"the killed CLI's turn is over: it will never send a turn_stop")

	// The incoming CLI opens its own turn; the outgoing one's late PTY death must not close it.
	f.term.duringTerminate = nil
	require.NoError(t, f.usecase.IngestHook(f.ctx, incoming, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "the new CLI is working"})))
	require.True(t, f.busy(t, chatID))

	f.term.exit(t, "term-1")
	f.wait()
	assert.True(t, f.busy(t, chatID),
		"the outgoing runner's exit must not close the incoming runner's turn")
}

// ---------------------------------------------------------------------------
// "" means NOWHERE, everywhere.
// ---------------------------------------------------------------------------

// TestRegression_HookFromAPurgedChatsRunner_NeverTouchesTheChatModel: a late hook
// from a purged chat's CLI resolves to no runner and so no chat; GetChat's lazy
// self-heal on a miss would replay the whole log for every dying CLI's hook.
func TestRegression_HookFromAPurgedChatsRunner_NeverTouchesTheChatModel(t *testing.T) {
	f, cs := newFaultFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	require.False(t, f.isLive(runnerID), "precondition: the purge ended the runner")

	cs.failGetChat = errors.New("boom: the chat model must not be consulted for nowhere")

	// Placed nowhere, it resolves to nowhere, without a lookup.
	lateHook(t, f, runnerID, "claude", "turn_stop", map[string]any{"last_assistant_message": "a dying CLI's last words"})
}

// TestRegression_APurgedChatKeepsNoRowOfItsRunnersExit: the purge records its
// runner's exit before it forgets the ledger, so the exit cannot write a row back
// into the chat it erased.
func TestRegression_APurgedChatKeepsNoRowOfItsRunnersExit(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	f.wait()

	rows, err := f.ledger.Read(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, rows, "a purged chat keeps no ledger row, its runner's exit included")
}

// mustLive reads the chat's live runner, failing the test if the chat is dormant.
func mustLive(t *testing.T, f testFixture, chatID string) engineagents.Runner {
	t.Helper()
	r, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	return r
}
