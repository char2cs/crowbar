package conversation_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"

	"github.com/char2cs/asynx"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/conversation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// The stores are the REAL asynx-backed aggregates over in-memory event logs, not
// stubs. Everything this package does is a write followed by a read of the same
// record, and a stub would let the two agree with each other while both were
// wrong.

type fixture struct {
	conversations *conversation.Conversations
	chats         agentchat.EventStore
	runners       agentrunner.EventStore
	retired       *retiredRunners
	home          string
	settle        func()
}

// retiredRunners is the runner lifecycle as a hard delete sees it: the one thing
// a purge owes the processes on the chat.
type retiredRunners struct {
	mu   sync.Mutex
	seen []string
}

func (r *retiredRunners) RetireChatRunners(_ context.Context, chatID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, chatID)
}

func (r *retiredRunners) all() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]string(nil), r.seen...)
}

// stubLineage answers the ancestry question without two tables behind it.
type stubLineage struct {
	ancestors []string
	err       error
}

func (s stubLineage) Ancestors(context.Context, string) ([]string, error) {
	return s.ancestors, s.err
}

// stubWorkspace roots the workspace strictly UNDER crowbar home, which mirrors a
// managed worktree and is load-bearing: every agent-path removal is guarded, and a
// chats dir outside home is refused.
type stubWorkspace struct{ home, worktree string }

func (w stubWorkspace) WorktreeDir(
	context.Context, string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	return w.home, "p1", "r1", w.worktree, nil
}

func (w stubWorkspace) AgentChatsDir(context.Context, string) (string, error) {
	return worktreepath.ChatsDir(w.worktree), nil
}

func newFixture(t *testing.T, lineage stubLineage) fixture {
	t.Helper()

	chats, waitChats := newChatStore(t)
	runners, waitRunners := newRunnerStore(t)
	activity, waitActivity := newActivityStore(t)

	home := t.TempDir()
	worktree := filepath.Join(home, "projects", "p1", "slug", "branch", "worktree")
	retired := &retiredRunners{}

	conversations := conversation.New(conversation.Deps{
		Chats:     chats,
		Runners:   runners,
		Activity:  activity,
		Telemetry: telemetry.New(),
		Agents:    engineagents.New(),
		Workspace: stubWorkspace{home: home, worktree: worktree},
		Lineage:   lineage,
		Home:      func() (string, error) { return home, nil },
		Work:      inflight.NewWork(),
		Spawns:    inflight.NewGate(),

		DefaultPermissionLevel: func(context.Context) (string, error) { return "guarded", nil },
	})
	conversations.SetRunners(retired)

	return fixture{
		conversations: conversations,
		chats:         chats,
		runners:       runners,
		retired:       retired,
		home:          home,
		settle:        func() { waitChats(); waitRunners(); waitActivity() },
	}
}

func newChatStore(t *testing.T) (agentchat.EventStore, func()) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	repo, err := agentchat.NewEventSourced(ax, es, db, nil)
	require.NoError(t, err)
	return repo, ax.WaitPublish
}

func newRunnerStore(t *testing.T) (agentrunner.EventStore, func()) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[engineagents.Runner]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	// The runner store REFUSES a nil watch: the fan-out seam is not optional.
	repo, err := agentrunner.NewEventSourced(ax, es, db, func(agentrunner.RunnerEvent) {})
	require.NoError(t, err)
	return repo, ax.WaitPublish
}

func newActivityStore(t *testing.T) (agentactivity.EventStore, func()) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.ChatActivity]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	repo, err := agentactivity.NewEventSourced(ax, es, db, t.TempDir())
	require.NoError(t, err)
	return repo, ax.WaitPublish
}

// A minted chat is DORMANT: it exists and is readable with no CLI in sight,
// which is the whole reason the record is separable from the runner lifecycle.
func TestMintChat_CreatesAReadableDormantChat(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	require.NotEmpty(t, chatID)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "ws-1", chat.WorkspaceID)
	assert.Empty(t, chat.Title)
	assert.False(t, chat.Working)
}

func TestListChatsByWorkspace_ScopesToTheWorkspace(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	mine, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	_, err = f.conversations.MintChat(t.Context(), "ws-2")
	require.NoError(t, err)
	f.settle()

	rows, err := f.conversations.ListChatsByWorkspace(t.Context(), "ws-1")

	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Equal(t, mine, rows[0].ID)

	all, err := f.conversations.ListChats(t.Context())
	require.NoError(t, err)
	assert.Len(t, all, 2, "the unscoped list still sees both")
}

// The precedence is user > agent > derived, and it is what stops a provider's
// guess overwriting a name the user typed.
func TestRenameChat_HonoursWhereTheTitleCameFrom(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)

	require.NoError(t, f.conversations.RenameChat(t.Context(), chatID, "first guess", "derived"))
	assert.Equal(t, "first guess", title(t, f, chatID))

	require.NoError(t, f.conversations.RenameChat(t.Context(), chatID, "second guess", "derived"))
	assert.Equal(t, "first guess", title(t, f, chatID),
		"a derived title never overwrites a title the chat already has")

	require.NoError(t, f.conversations.RenameChat(t.Context(), chatID, "the agent's name", "agent"))
	assert.Equal(t, "the agent's name", title(t, f, chatID))

	require.NoError(t, f.conversations.RenameChat(t.Context(), chatID, "what the user typed", "user"))
	assert.Equal(t, "what the user typed", title(t, f, chatID))

	require.NoError(t, f.conversations.RenameChat(t.Context(), chatID, "the agent again", "agent"))
	assert.Equal(t, "what the user typed", title(t, f, chatID),
		"a manual rename locks the title against the agent")
}

func TestRenameChat_AnEmptyTitleIsANoOp(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	require.NoError(t, f.conversations.RenameChat(t.Context(), chatID, "kept", "user"))

	require.NoError(t, f.conversations.RenameChat(t.Context(), chatID, "", "user"))

	assert.Equal(t, "kept", title(t, f, chatID))
}

func TestRenameByRunner_ARunnerPlacedNowhereRenamesNothing(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	err := f.conversations.RenameByRunner(t.Context(), "runner-nobody", "a title", "agent")

	require.Error(t, err, "an unknown runner is a lookup failure, not a silent no-op")
}

// A hard delete owes the processes on the chat exactly one thing: retirement.
// The record decides the chat must go; it never learns how a CLI is torn down.
func TestPurgeChat_ErasesTheChatAndRetiresItsRunnersThroughThePort(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	f.settle()

	require.NoError(t, f.conversations.PurgeChat(t.Context(), chatID))

	assert.Equal(t, []string{chatID}, f.retired.all(),
		"the CLIs on a purged chat must be retired, and only through the port")
	_, err = f.conversations.GetChat(t.Context(), chatID)
	require.Error(t, err, "a purged chat is gone from every read, including a direct get by id")
}

func TestPurgeChat_OfAnUnknownChatFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	err := f.conversations.PurgeChat(t.Context(), "never-minted")

	require.Error(t, err)
	assert.Empty(t, f.retired.all(), "nothing was retired for a chat that never existed")
}

func TestAncestors_AnswersThroughTheLineagePort(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{ancestors: []string{"parent", "grandparent"}})

	got, err := f.conversations.Ancestors(t.Context(), "chat-1")

	require.NoError(t, err)
	assert.Equal(t, []string{"parent", "grandparent"}, got)
}

func TestReadMessages_RefusesTwoCursorsAtOnce(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)

	_, err = f.conversations.ReadMessages(t.Context(), chatID, 5, 5, 10)

	require.Error(t, err, "after and before are mutually exclusive")
}

func TestReadMessages_RefusesAnOversizedPage(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)

	_, err = f.conversations.ReadMessages(t.Context(), chatID, 0, 0, 10_000)

	require.Error(t, err, "a caller must not be able to pull a whole conversation in one response")
}

func TestReadMessages_DefaultsThePageSize(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	f.settle()

	page, err := f.conversations.ReadMessages(t.Context(), chatID, 0, 0, 0)

	require.NoError(t, err)
	assert.Empty(t, page.Items, "a chat nobody has spoken in has no turns")
}

// The selection is validated against the provider's OWN declared catalogue, so a
// model no descriptor offers cannot be pinned to a chat.
func TestSetChatSelection_RefusesAModelNoProviderDeclares(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)

	err = f.conversations.SetChatSelection(t.Context(), chatID, "a-model-nobody-ships", "")

	require.Error(t, err)
}

func TestSetChatSelection_ClearingBackToTheProviderDefaultIsAllowed(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)

	require.NoError(t, f.conversations.SetChatSelection(t.Context(), chatID, "", ""))
	f.settle()

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Empty(t, chat.Model)
	assert.Empty(t, chat.Effort)
}

// The note is for a conversation ALREADY under way: a chat that has said nothing
// has nothing to be told it inherits, because the spawn is about to tell the CLI
// the same thing through its prior context.
func TestNoteThreadLineage_WritesNothingIntoAChatThatHasNotSpoken(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)

	require.NoError(t, f.conversations.NoteThreadLineage(t.Context(), chatID, []string{"parent-1"}))
	f.settle()

	turns, err := f.conversations.ChatTurns(t.Context(), chatID)
	require.NoError(t, err)
	assert.Empty(t, turns)
}

func TestNoteThreadLineage_AppendsTheNoteToAChatAlreadyUnderWay(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	require.NoError(t, f.conversations.RecordTurn(
		t.Context(), chat, "claude", "runner-1", "session-1", "user", "hello", "",
	))
	f.settle()

	require.NoError(t, f.conversations.NoteThreadLineage(t.Context(), chatID, []string{"parent-1"}))
	f.settle()

	turns, err := f.conversations.ChatTurns(t.Context(), chatID)
	require.NoError(t, err)
	require.Len(t, turns, 2)
	assert.Contains(t, turns[1].Text, "parent-1",
		"the note names what the thread reads, in the thread's own log")
}

func TestMintChat_SeedsThePermissionLevelFromTheCurrentGlobalDefault(t *testing.T) {
	t.Parallel()
	chats, waitChats := newChatStore(t)
	runners, _ := newRunnerStore(t)
	activity, _ := newActivityStore(t)
	home := t.TempDir()

	conversations := conversation.New(conversation.Deps{
		Chats:     chats,
		Runners:   runners,
		Activity:  activity,
		Telemetry: telemetry.New(),
		Agents:    engineagents.New(),
		Workspace: stubWorkspace{home: home, worktree: filepath.Join(home, "projects", "p1", "slug", "branch", "worktree")},
		Lineage:   stubLineage{},
		Home:      func() (string, error) { return home, nil },
		Work:      inflight.NewWork(),
		Spawns:    inflight.NewGate(),

		DefaultPermissionLevel: func(context.Context) (string, error) { return "trusted", nil },
	})

	chatID, err := conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	waitChats()

	chat, err := conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "trusted", chat.PermissionLevel)
}

func TestMintChat_ChangingTheGlobalDefaultDoesNotRetroactivelyChangeAnAlreadyOpenChat(t *testing.T) {
	t.Parallel()
	chats, waitChats := newChatStore(t)
	runners, _ := newRunnerStore(t)
	activity, _ := newActivityStore(t)
	home := t.TempDir()
	current := "guarded"

	conversations := conversation.New(conversation.Deps{
		Chats:     chats,
		Runners:   runners,
		Activity:  activity,
		Telemetry: telemetry.New(),
		Agents:    engineagents.New(),
		Workspace: stubWorkspace{home: home, worktree: filepath.Join(home, "projects", "p1", "slug", "branch", "worktree")},
		Lineage:   stubLineage{},
		Home:      func() (string, error) { return home, nil },
		Work:      inflight.NewWork(),
		Spawns:    inflight.NewGate(),

		DefaultPermissionLevel: func(context.Context) (string, error) { return current, nil },
	})

	chatID, err := conversations.MintChat(t.Context(), "ws-1")
	require.NoError(t, err)
	waitChats()
	chat, err := conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	require.Equal(t, "guarded", chat.PermissionLevel)

	current = "full-auto" // the global default changes AFTER this chat was minted

	chat, err = conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "guarded", chat.PermissionLevel,
		"an already-open chat's level must not drift when the global default later changes")
}

// title reads the chat's projected title, settling the read model first. The
// projection is asynchronous by design — the turn commands are on asynx's async
// Send path — so a read taken without settling is racing the write, not testing it.
func title(t *testing.T, f fixture, chatID string) string {
	t.Helper()
	f.settle()
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	return chat.Title
}
