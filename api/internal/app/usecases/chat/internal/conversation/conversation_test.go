package conversation_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/conversation"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/ingress"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/telemetry"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/transcript"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// The stores are the REAL asynx-backed aggregates over in-memory event logs, not
// stubs. Everything this package does is a write followed by a read of the same
// record, and a stub would let the two agree with each other while both were
// wrong.

type fixture struct {
	conversations *conversation.Conversations
	chats         agentchat.EventStore
	retired       *retiredRunners
	ledger        *ledgerstore.Store
	blobs         *ledgerstore.Blobs
	home          string
	settle        func()
	defaultLevel  *string // the global permission level a chat is minted under
}

// retiredRunners is the runner lifecycle with no CLI live: a purge owes the
// processes on the chat retirement, which it records with the chat's hold.
type retiredRunners struct {
	mu   sync.Mutex
	seen []string
}

func (*retiredRunners) RunnerByID(string) (engineagents.Runner, bool) {
	return engineagents.Runner{}, false
}

func (*retiredRunners) LiveRunnerForChat(context.Context, string) (engineagents.Runner, error) {
	return engineagents.Runner{}, errors.New("dormant")
}

func (r *retiredRunners) RetireChatRunners(_ context.Context, chatID string) func() {
	r.note("retire " + chatID)
	return func() { r.note("end " + chatID) }
}

func (r *retiredRunners) HoldChat(_ context.Context, chatID string) (func(), error) {
	return func() { r.note("release " + chatID) }, nil
}

func (r *retiredRunners) note(what string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.seen = append(r.seen, what)
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

	home := t.TempDir()
	worktree := filepath.Join(home, "projects", "p1", "slug", "branch", "worktree")
	retired := &retiredRunners{}
	chatLedger, projector := newLedger(t)
	blobs, err := ledgerstore.OpenBlobs(t.TempDir())
	require.NoError(t, err)
	level := "guarded"

	conversations := conversation.New(conversation.Deps{
		Chats:      chats,
		Transcript: projector,
		Blobs:      blobs,
		Telemetry:  telemetry.New(),
		Agents:     engineagents.New(),
		Workspace:  stubWorkspace{home: home, worktree: worktree},
		Lineage:    lineage,
		Home:       func() (string, error) { return home, nil },
		Ledger:     chatLedger, Writes: ingress.NewWriter(chatLedger, ingress.NewLocks()),

		DefaultPermissionLevel: func(context.Context) (string, error) { return level, nil },
	})
	conversations.SetRunners(retired)

	return fixture{
		conversations: conversations,
		chats:         chats,
		retired:       retired,
		ledger:        chatLedger,
		blobs:         blobs,
		home:          home,
		settle:        waitChats,
		defaultLevel:  &level,
	}
}

// newLedger is a ledger whose clock ticks a millisecond a commit, so rows are
// ordered in time as in sequence, and the transcript it publishes to.
func newLedger(t *testing.T) (*ledgerstore.Store, *transcript.Projector) {
	t.Helper()
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	var mu sync.Mutex
	now := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	chatLedger, err := ledgerstore.Open(db, func() time.Time {
		mu.Lock()
		defer mu.Unlock()
		now = now.Add(time.Millisecond)
		return now
	})
	require.NoError(t, err)
	projector := transcript.New(chatLedger)
	chatLedger.Subscribe(func(chatID string, rows []ledger.Row, _ ledger.State) { projector.Apply(chatID, rows) })
	return chatLedger, projector
}

// say records text as a person's prompt the CLI took, opening its turn.
func say(t *testing.T, f fixture, chatID, text string) {
	t.Helper()
	turnID := uuid.NewString()
	prompt, err := ledger.NewRow(ledger.KindUserMessage, turnID, "", ledger.UserMessage{Text: text, Origin: ledger.OriginProvider})
	require.NoError(t, err)
	started, err := ledger.NewRow(ledger.KindTurnStarted, turnID, "", ledger.TurnStarted{Provider: "claude", RunnerID: "runner-1"})
	require.NoError(t, err)
	_, _, err = f.ledger.Append(t.Context(), chatID, prompt, started)
	require.NoError(t, err)
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

// A minted chat is DORMANT: it exists and is readable with no CLI in sight,
// which is the whole reason the record is separable from the runner lifecycle.
func TestMintChat_CreatesAReadableDormantChat(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)
	require.NotEmpty(t, chatID)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "ws-1", chat.WorkspaceID)
	assert.Empty(t, chat.Title)
}

// A chat records the provider it is born on at its mint, before any CLI runs:
// a dormant chat with no switch marker has nothing else that names it.
func TestRegression_MintChat_RecordsTheProviderItIsBornOn(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "quietvendor", "")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "quietvendor", chat.ProviderID,
		"no CLI has run yet, and the chat can already say what it runs")
}

// A row minted with no vendor in mind — a placeholder that will never carry a
// CLI — records none, so "" stays the distinguishable answer for a chat that has
// genuinely never had a provider rather than one whose provider was lost.
func TestMintChat_NoProviderAskedForRecordsNone(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Empty(t, chat.ProviderID)
}

// The landing surface is DURABLE, not a one-shot spawn argument: every
// respawn (a restart_tui prompt, a model change, a resume after the daemon
// restarted) rebuilds the process from scratch, and a chat born on the
// terminal that silently came back on the api transport would lose the very
// view it was created for.
func TestMintChat_PersistsTheLandingSurface(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "terminal")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "terminal", chat.Surface)
}

// Omitting it stays the provider's own default landing — byte-identical to
// every chat minted before this field existed.
func TestMintChat_NoSurfaceAskedForStaysTheProvidersDefault(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Empty(t, chat.Surface)
}

func TestListChatsByWorkspace_ScopesToTheWorkspace(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	mine, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)
	_, err = f.conversations.MintChat(t.Context(), "ws-2", "", "")
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
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
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
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)
	require.NoError(t, f.conversations.RenameChat(t.Context(), chatID, "kept", "user"))

	require.NoError(t, f.conversations.RenameChat(t.Context(), chatID, "", "user"))

	assert.Equal(t, "kept", title(t, f, chatID))
}

func TestRenameByRunner_ARunnerThatIsNotLiveRenamesNothing(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	err := f.conversations.RenameByRunner(t.Context(), "runner-nobody", "a title", "agent")

	require.Error(t, err, "an unknown runner is a lookup failure, not a silent no-op")
}

// A hard delete owes the processes on the chat exactly one thing: retirement.
// The record decides the chat must go; it never learns how a CLI is torn down.
// Its ledger goes with it, so the user's words outlive the chat nowhere.
func TestPurgeChat_ErasesTheChatAndRetiresItsRunnersThroughThePort(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)
	f.settle()
	said, err := ledger.NewRow(ledger.KindUserMessage, "T1", "", ledger.UserMessage{Text: "secret", Origin: ledger.OriginProvider})
	require.NoError(t, err)
	_, _, err = f.ledger.Append(t.Context(), chatID, said)
	require.NoError(t, err)

	require.NoError(t, f.conversations.PurgeChat(t.Context(), chatID))
	rows, err := f.ledger.Read(t.Context(), chatID, 0, 0)
	require.NoError(t, err)
	assert.Empty(t, rows, "a purged chat leaves no ledger row behind")

	assert.Equal(t, []string{"retire " + chatID, "release " + chatID, "end " + chatID}, f.retired.all(),
		"the CLIs on a purged chat are retired through the port, their processes ended once the chat is let go")
	_, err = f.conversations.GetChat(t.Context(), chatID)
	require.Error(t, err, "a purged chat is gone from every read, including a direct get by id")
}

// A purged chat's tool payloads go with its rows, except one another chat's
// rows still name.
func TestPurgeChat_DeletesThePayloadsOnlyItsRowsName(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)
	f.settle()
	own, err := f.blobs.Put([]byte("only this chat's output"))
	require.NoError(t, err)
	shared, err := f.blobs.Put([]byte("output two chats name"))
	require.NoError(t, err)
	call := func(chat, toolID, ref string) {
		row, err := ledger.NewRow(ledger.KindToolCall, "T1", "", ledger.ToolCall{
			ToolID: toolID, Phase: ledger.ToolPost, Status: domain.ToolStatusOK, ResultRef: ref,
		})
		require.NoError(t, err)
		_, _, err = f.ledger.Append(t.Context(), chat, row)
		require.NoError(t, err)
	}
	call(chatID, "tool-1", own)
	call(chatID, "tool-2", shared)
	call("other-chat", "tool-3", shared)

	require.NoError(t, f.conversations.PurgeChat(t.Context(), chatID))

	_, err = f.blobs.Get(own)
	require.Error(t, err, "a payload only the purged chat named is erased with it")
	kept, err := f.blobs.Get(shared)
	require.NoError(t, err, "a payload another chat still names survives")
	assert.Equal(t, "output two chats name", string(kept))
}

func TestPurgeChat_OfAnUnknownChatFails(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})

	err := f.conversations.PurgeChat(t.Context(), "never-minted")

	require.Error(t, err)
	assert.Equal(t, []string{"release never-minted"}, f.retired.all(), "nothing was retired for a chat that never existed")
}

func TestAncestors_AnswersThroughTheLineagePort(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{ancestors: []string{"parent", "grandparent"}})

	got, err := f.conversations.Ancestors(t.Context(), "chat-1")

	require.NoError(t, err)
	assert.Equal(t, []string{"parent", "grandparent"}, got)
}

func TestReadMessages_RefusesAPageItCannotServe(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)

	_, err = f.conversations.ReadMessages(t.Context(), chatID, 5, 5, 10)
	require.Error(t, err, "after and before are mutually exclusive")
	_, err = f.conversations.ReadMessages(t.Context(), chatID, 0, 0, 10_000)
	require.Error(t, err, "a caller must not be able to pull a whole conversation in one response")
}

func TestReadMessages_DefaultsThePageSize(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
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
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)

	err = f.conversations.SetChatSelection(t.Context(), chatID, "a-model-nobody-ships", "")

	require.Error(t, err)
}

func TestSetChatSelection_ClearingBackToTheProviderDefaultIsAllowed(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
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
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
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
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)
	say(t, f, chatID, "hello")

	require.NoError(t, f.conversations.NoteThreadLineage(t.Context(), chatID, []string{"parent-1"}))
	f.settle()

	turns, err := f.conversations.ChatTurns(t.Context(), chatID)
	require.NoError(t, err)
	require.Len(t, turns, 2)
	assert.Contains(t, turns[1].Text, "parent-1",
		"the note names what the thread reads, in the thread's own log")
}

func TestNotePromotion_WritesNothingIntoAChatThatHasNotSpoken(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)

	require.NoError(t, f.conversations.NotePromotion(t.Context(), chatID))
	f.settle()

	turns, err := f.conversations.ChatTurns(t.Context(), chatID)
	require.NoError(t, err)
	assert.Empty(t, turns)
}

func TestNotePromotion_AppendsTheNoteToAChatAlreadyUnderWay(t *testing.T) {
	t.Parallel()

	f := newFixture(t, stubLineage{})
	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)
	say(t, f, chatID, "hello")

	require.NoError(t, f.conversations.NotePromotion(t.Context(), chatID))
	f.settle()

	turns, err := f.conversations.ChatTurns(t.Context(), chatID)
	require.NoError(t, err)
	require.Len(t, turns, 2)
	assert.Contains(t, turns[1].Text, "promoted",
		"the note describes the promotion, in the chat's own log")
}

func TestMintChat_SeedsThePermissionLevelFromTheCurrentGlobalDefault(t *testing.T) {
	t.Parallel()
	f := newFixture(t, stubLineage{})
	*f.defaultLevel = "trusted"

	chatID, err := f.conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)
	f.settle()

	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "trusted", chat.PermissionLevel)
}

// A chat's stored level is the display snapshot taken at mint; a chat never
// explicitly pinned spawns under the CURRENT global default, which its
// selection resolves at every spawn.
func TestMintChat_TheSeededFieldIsAFrozenSnapshotButChatSelectionTracksTheLiveDefault(t *testing.T) {
	t.Parallel()
	f := newFixture(t, stubLineage{})
	conversations := f.conversations

	chatID, err := conversations.MintChat(t.Context(), "ws-1", "", "")
	require.NoError(t, err)
	f.settle()
	chat, err := conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	require.Equal(t, "guarded", chat.PermissionLevel)
	require.False(t, chat.PermissionLevelExplicit, "a plain mint is never an explicit pick")

	*f.defaultLevel = "full-auto" // the global default changes AFTER this chat was minted

	chat, err = conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	assert.Equal(t, "guarded", chat.PermissionLevel,
		"the STORED/display field is a snapshot taken at mint and must not drift on its own")

	sel, err := conversations.ChatSelection(t.Context(), chatID, false)
	require.NoError(t, err)
	assert.Equal(t, "full-auto", sel.PermissionLevel,
		"but a chat that was never explicitly pinned must SPAWN under the CURRENT global "+
			"default, not the value frozen in the field above — this is the actual fix")
}

// title reads the chat's projected title once its read model has settled.
func title(t *testing.T, f fixture, chatID string) string {
	t.Helper()
	f.settle()
	chat, err := f.conversations.GetChat(t.Context(), chatID)
	require.NoError(t, err)
	return chat.Title
}
