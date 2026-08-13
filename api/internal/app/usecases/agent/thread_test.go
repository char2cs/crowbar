package agent_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/domain"
)

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
	require.NoError(t, f.folders.Save(f.ctx, domain.AgentChatFolder{
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

func TestNoteThreadLineage_SurfacesALedgerOpenFailure(t *testing.T) {
	f := newFixture(t)

	chatID, _ := f.spawn(t, "claude")
	f.ws.err = errors.New("chats dir unreadable")

	require.ErrorContains(t, f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1"}),
		"chats dir unreadable")
}

// A ledger that cannot be READ is not an empty one, so it must not be treated as
// the "nothing to date" case — the note would be silently dropped for a chat that
// has a whole history.
func TestNoteThreadLineage_SurfacesAnUnreadableLedger(t *testing.T) {
	f := newFixture(t)

	chatID, runnerID := f.spawn(t, "claude")
	turn(t, f, runnerID, "claude", "something was said here")
	require.NoError(t, os.WriteFile(
		filepath.Join(f.ws.chatsDir, chatID, "ledger", "99999999-corrupt-user-claude.turn"),
		[]byte("{not json"), 0o600))

	require.Error(t, f.usecase.NoteThreadLineage(f.ctx, chatID, []string{"parent-1"}))
}
