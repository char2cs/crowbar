package chat_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"gopkg.in/yaml.v3"

	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func getTitle(t *testing.T, f testFixture, chatID string) string {
	t.Helper()
	c := f.chat(t, chatID)
	return c.Title
}

func TestRenameChat_Precedence(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.spawnChat(ctx, "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "First Topic", "derived"))
	assert.Equal(t, "First Topic", getTitle(t, f, chatID))
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Second Topic", "derived"))
	assert.Equal(t, "First Topic", getTitle(t, f, chatID))

	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Agent Title", "agent"))
	assert.Equal(t, "Agent Title", getTitle(t, f, chatID))

	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "User Title", "user"))
	assert.Equal(t, "User Title", getTitle(t, f, chatID))
	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "Agent Again", "agent"))
	assert.Equal(t, "User Title", getTitle(t, f, chatID))

	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "", "user"))
	assert.Equal(t, "User Title", getTitle(t, f, chatID))
}

func TestRenameChat_BroadcastsTitleSetOnSuccessfulChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, _, err := f.spawnChat(ctx, "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset()

	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "A Title", "user"))
	assert.Equal(t, []string{"title_set"}, f.bcKinds(t))

	require.NoError(t, f.usecase.RenameChat(ctx, chatID, "", "user"))
	assert.Equal(t, []string{"title_set"}, f.bcKinds(t))

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

func TestRenameByRunner_UnknownRunner_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.RenameByRunner(ctx, "does-not-exist", "Some Title", "agent")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rename by runner")
}

func TestIngestHook_UserPrompt_SetsDerivedTitle(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.spawnChat(ctx, "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "Refactor the auth module to use JWT\nmore detail"})))
	assert.Equal(t, "Refactor the auth module to use JWT", getTitle(t, f, chatID))

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "now do something else"})))
	assert.Equal(t, "Refactor the auth module to use JWT", getTitle(t, f, chatID))
}

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

func TestRegression_DeleteChatMidTurn_DoesNotResurrectTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "working on it"})))
	require.True(t, f.busy(t, chatID), "precondition: the chat is mid-turn when it is deleted")

	termSess := f.runner(t, runnerID).TerminalSession
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

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

func TestPurgeChat_DormantChat_NothingToTerminate(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.term.exit(t, f.runner(t, runnerID).TerminalSession)
	f.wait()

	before := len(f.term.terminateRequestIDs())
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	assert.Len(t, f.term.terminateRequestIDs(), before, "a dormant chat has no CLI to terminate")
	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

func TestPurgeChat_TerminateFailure_SessionAlreadyGone_ContinuesPurge(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.term.terminateErr = fmt.Errorf("terminal: terminate: %w: term-1", engineterminal.ErrSessionNotFound)

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound)
}

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

func TestPurgeChat_ReapsChatDirOnDisk(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	chatDir := filepath.Join(worktreepath.LedgerChatsDir(f.ws.home), chatID)
	require.NoError(t, os.MkdirAll(chatDir, 0o700))
	turn(t, f, runnerID, "claude", "a turn, so the chat has a conversation to purge")

	before, err := f.activity.Turns(f.ctx, chatID)
	require.NoError(t, err)
	require.NotEmpty(t, before, "precondition: the chat has a recorded conversation")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	after, err := f.ledger.Read(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, after, "purge must drop the chat's conversation record")

	_, err = os.Stat(chatDir)
	assert.True(t, os.IsNotExist(err), "purge must reap the chat's on-disk dir")
}

func TestPurgeChat_ReapFailure_StillPurges(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	*f.homeErr = errors.New("boom: crowbar home lookup for reap")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID), "a reap-path failure must not abort the purge")

	_, err := f.usecase.GetChat(f.ctx, chatID)
	assert.ErrorIs(t, err, agentchat.ErrNotFound, "the aggregate is still Forgotten")
}

func TestPurgeChat_UnknownChat_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.PurgeChat(f.ctx, "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "purge chat: get")
}

func TestPurgeChat_UnknownChat_AlsoAnswersApperrNotFound(t *testing.T) {
	f := newFixture(t)

	err := f.usecase.PurgeChat(f.ctx, "does-not-exist")
	require.Error(t, err)
	require.ErrorIs(t, err, problem.ErrNotFound)
	assert.ErrorIs(t, err, agentchat.ErrNotFound, "the original cause stays inspectable too")
}

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

func injectedContext(
	t *testing.T,
	f testFixture,
	spawn int,
) string {
	t.Helper()
	argv := f.term.calls[spawn].argv
	if at := slices.Index(argv, "--append-system-prompt"); at >= 0 && at+1 < len(argv) {
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

func file(
	t *testing.T,
	f testFixture,
	id string,
	parentID string,
) {
	t.Helper()
	f.folders.Saved = append(f.folders.Saved, tree.Folder{ID: id, Name: id})
	_, err := f.nodes.Create(f.ctx, id, tree.KindFolder, parentID, 0)
	require.NoError(t, err)
}

func lineageBlock(
	ids ...string,
) string {
	lines := make([]string, 0, len(ids))
	for _, id := range ids {
		lines = append(lines, "- "+id)
	}
	return strings.ReplaceAll(config.GetPrompts().ThreadLineage, "{lineage}", strings.Join(lines, "\n"))
}

func spawnPlaced(t *testing.T, f testFixture, parentID string) string {
	t.Helper()
	chatID, err := f.usecase.MintChat(f.ctx, "", "ws1", "claude", "")
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

func TestSpawn_ADeepThreadIsToldItsWholeChainNearestFirst(t *testing.T) {
	f := newFixture(t)

	grandparentID, _ := f.spawn(t, "claude")
	parentID, _ := f.spawn(t, "claude")
	thread(t, f, parentID, grandparentID)
	spawnPlaced(t, f, parentID)

	assert.Contains(t, injectedContext(t, f, 2), lineageBlock(parentID, grandparentID))
}

func TestSpawn_AChatWithNoChatAncestorsIsToldNothingExtra(t *testing.T) {
	f := newFixture(t)

	file(t, f, "somewhere", "")
	spawnPlaced(t, f, "")
	spawnPlaced(t, f, "somewhere")

	assert.Equal(t, injectedContext(t, f, 0), injectedContext(t, f, 1),
		"a chat merely filed in a folder inherits nothing and is spawned identically to an unfiled one")
	assert.NotContains(t, injectedContext(t, f, 1), "THREAD CONTEXT")
}

func TestSpawn_ALineageThatCannotBeReadFailsTheSpawn(t *testing.T) {
	f, cs := newFaultFixture(t)

	parentID, _ := f.spawn(t, "claude")
	threadID, _ := f.spawn(t, "claude")
	thread(t, f, threadID, parentID)
	cs.failListByWorkspace = errors.New("folder table unreadable")

	_, err := f.usecase.SwitchProvider(f.ctx, threadID, "claude", "")
	require.ErrorContains(t, err, "folder table unreadable")
	require.Len(t, f.term.calls, 2, "and no CLI was started")
}

func TestSpawn_MintingAChatResolvesNoLineage(t *testing.T) {
	f, cs := newFaultFixture(t)
	cs.failListByWorkspace = errors.New("folder table unreadable")

	_, _, err := f.spawnChat(f.ctx, "claude")
	require.NoError(t, err)
}

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

func TestNoteThreadLineage_IsNotAttributedToAnyProvider(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	f.announce(t, runnerID, "sid-claude")
	require.NoError(t, f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1"}))
	f.wait()

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	f.wait()
	_, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude", "")
	require.NoError(t, err)
	f.wait()
	assert.Equal(t, -1, slices.Index(f.term.calls[2].argv, "--resume"), "claude never spoke in sid-claude")
}

func TestMintChat_CreatesTheChatAndNoRunner(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "", "ws1", "", "")
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

	_, err := f.usecase.MintChat(f.ctx, "", "ws1", "", "")
	require.ErrorContains(t, err, "chat store down")
}

func TestStartRunner_ATheadPlacedBeforeItsFirstSpawnIsToldItsLineage(t *testing.T) {
	f := newFixture(t)

	parentID, _ := f.spawn(t, "claude")
	threadID, err := f.usecase.MintChat(f.ctx, "", "ws1", "", "")
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

func TestStartRunner_AnUnplacedChatIsToldNothingExtra(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "", "ws1", "", "")
	require.NoError(t, err)
	f.wait()

	_, err = f.usecase.StartRunner(f.ctx, chatID, "claude")
	require.NoError(t, err)
	f.wait()

	assert.NotContains(t, injectedContext(t, f, 0), "THREAD CONTEXT")
}

func TestStartRunner_UnknownChat_StartsNothing(t *testing.T) {
	f := newFixture(t)

	_, err := f.usecase.StartRunner(f.ctx, "no-such-chat", "claude")
	require.Error(t, err)
	assert.Empty(t, f.term.calls)
}

func TestNoteThreadLineage_SaysNothingInAChatThatHasNotSpoken(t *testing.T) {
	f := newFixture(t)

	chatID, err := f.usecase.MintChat(f.ctx, "", "ws1", "", "")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1"}))

	turns, err := f.usecase.ReadChatLog(f.ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, turns, "a chat with no history has no move to date")
}

func TestStartRunner_ThreadsAChatTheReadModelStillReportsUnplaced(t *testing.T) {
	f, chatStore := newFaultFixture(t)

	parentID, _ := f.spawn(t, "claude")
	threadID, err := f.usecase.MintChat(f.ctx, "", "ws1", "", "")
	require.NoError(t, err)
	f.wait()
	thread(t, f, threadID, parentID)

	chatStore.staleProjection = true

	_, err = f.usecase.StartRunner(f.ctx, threadID, "claude")
	require.NoError(t, err)
	f.wait()

	assert.Contains(t, injectedContext(t, f, 1), lineageBlock(parentID),
		"the spawn must resolve placement from the event log, never from the projection it just outran")
}

func TestAssembleHandoff_WrapsLedgerEntriesInPreamble(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	turn(t, f, runnerID, "claude", "first turn transcript")
	turn(t, f, runnerID, "claude", "second turn transcript")

	got, err := f.usecase.AssembleHandoff(f.ctx, chatID)
	require.NoError(t, err)

	wrapper := config.GetPrompts().HandoffWrapper
	pre, post, ok := strings.Cut(wrapper, "{conversation}")
	require.True(t, ok, "handoff_wrapper must contain {conversation}")
	assert.True(t, strings.HasPrefix(got, pre))
	assert.True(t, strings.HasSuffix(got, post))
	assert.Contains(t, got, "first turn transcript")
	assert.Contains(t, got, "second turn transcript")
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

func resumeClaudeWithGap(t *testing.T, f testFixture) (chatID, claudeRunnerID, injected string) {
	t.Helper()

	chatID, claudeRunner := f.spawn(t, "claude")
	f.announce(t, claudeRunner, "sid-claude")
	exchange(t, f, claudeRunner, "claude", "claude said this before leaving")

	codexRunner, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	f.wait()
	f.announce(t, codexRunner, "sid-codex-away")
	codexTurnStop(t, f, codexRunner, "sid-codex-away", "codex spoke while claude was away")

	claudeRunnerID, err = f.usecase.SwitchProvider(f.ctx, chatID, "claude", "")
	require.NoError(t, err)
	f.wait()

	argv := f.term.calls[2].argv
	injected = argv[len(argv)-1]

	require.Contains(t, injected, "codex spoke while claude was away",
		"the resumed claude must be handed the real gap, not a pointer to fetch later: %v", argv)
	require.NotContains(t, injected, "claude said this before leaving",
		"a provider resumed into its own conversation must not be re-fed its own earlier turns")
	require.True(t, strings.HasPrefix(injected, "<system-reminder>") && strings.HasSuffix(injected, "</system-reminder>"),
		"the gap must be wrapped as trusted context, not delivered as a bare user turn: %q", injected)
	return chatID, claudeRunnerID, injected
}

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

func TestResumeClaude_InjectedPointer_StillOpensTheTurn(t *testing.T) {
	f := newFixture(t)

	chatID, claudeRunner, injected := resumeClaudeWithGap(t, f)

	require.NoError(t, f.usecase.IngestHook(f.ctx, claudeRunner, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": injected, "session_id": "sid-claude"})))
	f.wait()

	assert.True(t, f.busy(t, chatID), "the CLI is answering the gap: the chat must read as working")
}

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

func TestRunnerExit_ForgetsTheInjectedContext(t *testing.T) {
	f := newFixture(t)

	_, claudeRunner, injected := resumeClaudeWithGap(t, f)

	_, found := f.engine.ConsumeInjectedPrefix(claudeRunner, injected)
	require.True(t, found)

	f.engine.RecordInjection(claudeRunner, injected)
	f.term.exit(t, f.runner(t, claudeRunner).TerminalSession)
	f.wait()

	_, found = f.engine.ConsumeInjectedPrefix(claudeRunner, injected)
	assert.False(t, found, "a dead runner's injected context must be forgotten")
}

func writeDescriptor(t *testing.T, f testFixture, id, body string) {
	t.Helper()
	dir := filepath.Join(f.ws.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(dir, id+".yaml"), []byte(withLifecycle(body)), 0o600))
}

func withLifecycle(body string) string {
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(body), &doc); err != nil || doc == nil {
		return body
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
			"arg": "--hook", "value": "{hook_curl} {hook_url}",
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

func TestSetChatSelection_RecordsChatSwitchInterruptions(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	exchange(t, f, runnerID, "claude", "a chat that has said something")

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

			require.ErrorIs(t, err, problem.ErrInvalidArgument)
			assert.Contains(t, err.Error(), tc.wantMsg)
			assert.Empty(t, f.chat(t, chatID).Model, "a refused write must store nothing")
		})
	}
}

func TestRegression_SetChatSelection_ConcurrentStandaloneAndStagedNeverLogAStaleChange(t *testing.T) {
	f := newFixture(t)
	chatID, runnerID := f.spawn(t, "claude")
	exchange(t, f, runnerID, "claude", "a chat that has said something")

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

	require.ErrorIs(t, err, problem.ErrInvalidArgument)
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

	require.ErrorIs(t, err, problem.ErrInvalidArgument)
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
	assert.Equal(t, "opus", argAfter(t, call.argv, "--model"))
	assert.Equal(t, "high", argAfter(t, call.argv, "--effort"))

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "opus", live.LaunchModel)
	assert.Equal(t, "high", live.LaunchEffort)
}

func TestSubmitPrompt_CommitsAStagedSelectionItselfBeforeSpawning(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "", &domain.ChatSelection{Model: "opus", Effort: "high"})
	require.NoError(t, err)

	call := f.term.calls[f.term.callCount()-1]
	assert.Equal(t, "opus", argAfter(t, call.argv, "--model"))
	assert.Equal(t, "high", argAfter(t, call.argv, "--effort"))

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "opus", live.LaunchModel)
	assert.Equal(t, "high", live.LaunchEffort)
}

func TestSubmitPrompt_StagedProviderSwitchesBeforeDelivering(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "codex", nil)
	require.NoError(t, err)
	f.wait()

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, "codex", live.ProviderID, "the switch actually ran before delivery")

	ints, err := f.activity.Interruptions(f.ctx, chatID)
	require.NoError(t, err)
	require.Len(t, ints, 1)
	assert.Equal(t, ledger.MarkerProviderSwitched, ints[0].Kind)
	assert.Equal(t, "codex", ints[0].Detail)
}

func TestSubmitPrompt_SameProviderNeverTriggersASwitch(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	spawned := f.term.callCount()
	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "first", uuid.NewString(), "", nil)
	require.NoError(t, err)
	ordinaryDelta := f.term.callCount() - spawned

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
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

func TestSubmitPrompt_RefusesAndNeverSpawnsOnAnInvalidStagedSelection(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	callsBefore := f.term.callCount()

	_, err := f.usecase.SubmitPrompt(f.ctx, chatID, "go", uuid.NewString(), "", &domain.ChatSelection{Model: "gpt-5", Effort: ""})

	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	assert.Equal(t, callsBefore, f.term.callCount(), "an invalid selection must not spawn a replacement runner")
}

type undeliverableAgent struct {
	engineagents.Agent
}

func (undeliverableAgent) Capabilities() engineagents.Capabilities {
	return engineagents.Capabilities{PromptSubmit: true, Delivery: "telepathy"}
}

func undeliverableClaude(t *testing.T, f testFixture, body string) (string, engineagents.Runner, undeliverableAgent) {
	t.Helper()
	writeDescriptor(t, f, "claude", body)
	chatID, _ := f.spawn(t, "claude")
	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	shipped, err := f.engine.Get(f.ctx, f.ws.home, "claude")
	require.NoError(t, err)
	descriptor := undeliverableAgent{Agent: shipped}
	require.NotEqual(t, engineagents.DeliveryRestartTUI, descriptor.Capabilities().Delivery,
		"the fixture must not restart for delivery reasons, or this proves nothing")
	return chatID, live, descriptor
}

func TestSubmitPrompt_ASelectionSwitchAuthorisesARestartADeliveryWouldNotHaveDone(t *testing.T) {
	f := newFixture(t)
	chatID, live, descriptor := undeliverableClaude(t, f, selectingDescriptorBody)

	err := agentusecase.RequirePromptRestart(f.ctx, f.usecase.RunnerUsecase, chatID, live, descriptor)
	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported,
		"a delivery this daemon has no channel for is refused, never guessed at")

	require.NoError(t, f.usecase.SetChatSelection(f.ctx, chatID, "opus", ""))

	require.NoError(t, agentusecase.RequirePromptRestart(f.ctx, f.usecase.RunnerUsecase, chatID, live, descriptor),
		"a pending selection switch obliges a restart on its own")
}

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

func TestRegression_PermissionLevelUnchangedDoesNotForceARestartADeliveryWouldNotHaveDone(t *testing.T) {
	f := newFixture(t)
	chatID, live, descriptor := undeliverableClaude(t, f, permissionSelectingDescriptorBody)
	require.Equal(t, "guarded", live.LaunchPermissionLevel,
		"precondition: spawned under the fixture's own pinned default")

	err := agentusecase.RequirePromptRestart(f.ctx, f.usecase.RunnerUsecase, chatID, live, descriptor)

	require.ErrorIs(t, err, agentusecase.ErrPromptUnsupported,
		"the chat's permission level never changed from what this runner launched under; "+
			"a delivery this daemon has no channel for must stay refused")
}

func TestSubmitPrompt_PermissionLevelSwitchAloneAuthorisesARestartADeliveryWouldNotHaveDone(t *testing.T) {
	f := newFixture(t)
	chatID, live, descriptor := undeliverableClaude(t, f, permissionSelectingDescriptorBody)

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
	assert.Equal(t, "native-session", argAfter(t, call.argv, "--resume"), "the forced restart must resume, not start fresh")
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
	assert.NotContains(t, call.argv, "--model", "the default carries no model flag")
}

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
	assert.NotContains(t, call.argv, "--model", "the default carries no model flag")
	assert.NotContains(t, call.argv, "--effort", "nor an effort one")
}

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

func TestRegression_SubmitPrompt_ReplayingAStagedSelectionStaysAtMostOnce(t *testing.T) {
	f := newFixture(t)
	writeDescriptor(t, f, "claude", selectingDescriptorBody)
	chatID, runnerID := f.spawn(t, "claude")
	exchange(t, f, runnerID, "claude", "a chat that has said something")
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

func unseededChat(t *testing.T, f testFixture) string {
	t.Helper()
	writeDescriptor(t, f, "claude", permissionSelectingDescriptorBody)
	chatID := uuid.NewString()
	_, err := f.chats.Create(f.ctx, agentchat.CreateInput{
		ID: chatID, WorkspaceID: "ws1", Type: domain.ChatTypeChat, Now: time.Now(),
	})
	require.NoError(t, err)
	f.wait()
	return chatID
}

func TestRegression_UnseededChatPermissionLevelSpawnsUnderGlobalDefaultNotGuarded(t *testing.T) {
	f := newFixtureWithPermissionDefault(t, "full-auto")
	chatID := unseededChat(t, f)
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

func TestRegression_UnseededChatHonoursAnExplicitLevelOnItsNextRestart(t *testing.T) {
	f := newFixtureWithPermissionDefault(t, "guarded")
	chatID := unseededChat(t, f)

	_, err := f.usecase.StartRunner(f.ctx, chatID, "claude")
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
	assert.Equal(t, []string{"fable", "opus", "sonnet", "haiku", "opusplan"}, claude.Models)
	assert.Equal(t, []string{"low", "medium", "high", "xhigh", "max"}, claude.Efforts[""],
		"the '' key is the union of every model's own levels — see modeldiscovery.effortsOf")
	for _, model := range claude.Models {
		assert.NotEmpty(t, claude.Efforts[model], "every selectable model must answer its levels")
	}
	assert.Empty(t, claude.DefaultModel, "claude's manifest states no default, so this is unknown")

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
	chatID, err := f.usecase.MintChat(f.ctx, "", "ws1", "", "")
	require.NoError(t, err)
	f.wait()

	err = f.usecase.SetChatSelection(f.ctx, chatID, "opus", "")

	require.ErrorIs(t, err, problem.ErrUnprocessable)

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

func TestRegression_SessionStartAfterItsChatWasDeleted_IsDroppedNotRecorded(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	termSess := f.runner(t, runnerID).TerminalSession

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))

	lateHook(t, f, runnerID, "", "session_start", map[string]any{"session_id": "s-late"})

	assert.Empty(t, f.chatForSession(t, "claude", "s-late"),
		"a conversation must never be recorded against a chat that has been deleted")

	assert.Contains(t, f.term.terminateRequestIDs(), termSess,
		"a CLI with nowhere left to write must not be left running")
}

func TestRegression_LateSessionAnnouncement_DoesNotStealTheChatFromTheIncomingCLI(t *testing.T) {
	f := newFixture(t)

	chatID, outgoing := f.spawn(t, "claude")

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	f.wait()

	hook(t, f, outgoing, "", "session_start", map[string]any{"session_id": "s-late"})

	live, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	assert.Equal(t, incoming, live.ID, "the chat belongs to the incoming CLI, never the corpse")
	assert.Equal(t, f.runner(t, incoming).TerminalSession, live.TerminalSession,
		"the pane must attach to the incoming CLI's PTY")

	assert.False(t, f.isLive(outgoing), "the outgoing CLI was off the chat before the switch returned")
}

func TestRegression_ConcurrentSwitches_LeaveExactlyOneCLIOnTheChat(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")

	var g errgroup.Group
	for _, target := range []string{"codex", "claude"} {
		g.Go(func() error {
			_, err := f.usecase.SwitchProvider(f.ctx, chatID, target, "")
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

func TestRegression_InvalidSwitchTarget_PreservesTheOutgoingRunner(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "not-a-real-provider", "")
	require.Error(t, err)
	f.wait()

	live, liveErr := f.liveRunnerFor(t, chatID)
	require.NoError(t, liveErr)
	assert.Equal(t, runnerID, live.ID)
	assert.Empty(t, f.term.terminatedIDs())
}

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

	incoming, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex", "")
	require.NoError(t, err)
	require.NoError(t, <-hookDone)
	f.wait()

	assert.False(t, f.busy(t, chatID),
		"the killed CLI's turn is over: it will never send a turn_stop")

	f.term.duringTerminate = nil
	require.NoError(t, f.usecase.IngestHook(f.ctx, incoming, "codex", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "the new CLI is working"})))
	require.True(t, f.busy(t, chatID))

	f.term.exit(t, "term-1")
	f.wait()
	assert.True(t, f.busy(t, chatID),
		"the outgoing runner's exit must not close the incoming runner's turn")
}

func TestRegression_HookFromAPurgedChatsRunner_NeverTouchesTheChatModel(t *testing.T) {
	f, cs := newFaultFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	require.False(t, f.isLive(runnerID), "precondition: the purge ended the runner")

	cs.failGetChat = errors.New("boom: the chat model must not be consulted for nowhere")

	lateHook(t, f, runnerID, "claude", "turn_stop", map[string]any{"last_assistant_message": "a dying CLI's last words"})
}

func TestRegression_APurgedChatKeepsNoRowOfItsRunnersExit(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")

	require.NoError(t, f.usecase.PurgeChat(f.ctx, chatID))
	f.wait()

	rows, err := f.ledger.Read(f.ctx, chatID, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, rows, "a purged chat keeps no ledger row, its runner's exit included")
}

func mustLive(t *testing.T, f testFixture, chatID string) engineagents.Runner {
	t.Helper()
	r, err := f.liveRunnerFor(t, chatID)
	require.NoError(t, err)
	return r
}
