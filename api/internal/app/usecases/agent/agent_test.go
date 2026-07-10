package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

// mustJSON marshals m to raw JSON bytes for IngestHook's rawPayload argument.
func mustJSON(t *testing.T, m map[string]any) []byte {
	t.Helper()
	b, err := json.Marshal(m)
	require.NoError(t, err)
	return b
}

type commandCall struct {
	workspaceID string
	cwd         string
	argv        []string
	env         []string
	onExit      func()
}

// fakeCommander is a thread-safe TerminalCommander double: CreateCommand records
// the spawn and hands back a unique "term-N" session id; TerminateGraceful
// records the id. The mutex makes it safe under the concurrent-switch tests
// (run with -race). deadSessions is the injected "is this terminal session
// still alive" fake: SessionExists defaults to true (alive) for any session id
// not explicitly marked dead via killSession, so tests that don't care about
// liveness (the vast majority) are unaffected.
type fakeCommander struct {
	mu           sync.Mutex
	calls        []commandCall
	terminated   []string
	terminateReq []string
	nextID       int
	err          error
	terminateErr error
	deadSessions map[string]bool
}

func (f *fakeCommander) CreateCommand(
	_ context.Context,
	workspaceID string,
	cwd string,
	argv []string,
	env []string,
	onExit func(),
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	f.nextID++
	f.calls = append(f.calls, commandCall{
		workspaceID: workspaceID,
		cwd:         cwd,
		argv:        append([]string{}, argv...),
		env:         append([]string{}, env...),
		onExit:      onExit,
	})
	return fmt.Sprintf("term-%d", f.nextID), nil
}

func (f *fakeCommander) TerminateGraceful(
	_ context.Context,
	sessionID string,
) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	// terminateReq records EVERY attempt (even failed ones), so a best-effort
	// caller can assert the terminate was attempted regardless of outcome;
	// terminated stays success-only for callers that only care about the ids
	// actually torn down.
	f.terminateReq = append(f.terminateReq, sessionID)
	if f.terminateErr != nil {
		return f.terminateErr
	}
	f.terminated = append(f.terminated, sessionID)
	return nil
}

// SessionExists implements the agent.TerminalCommander liveness seam:
// alive unless explicitly killed via killSession.
func (f *fakeCommander) SessionExists(_ context.Context, sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.deadSessions[sessionID]
}

// killSession marks sessionID as no longer live, so a subsequent
// SessionExists call reports it gone — the boot-reconcile fake for "the CLI
// process died with the daemon and never came back."
func (f *fakeCommander) killSession(sessionID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deadSessions == nil {
		f.deadSessions = map[string]bool{}
	}
	f.deadSessions[sessionID] = true
}

func (f *fakeCommander) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

func (f *fakeCommander) terminatedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.terminated...)
}

// terminateRequestIDs returns every session id TerminateGraceful was CALLED
// with, including attempts that returned an error (unlike terminatedIDs, which
// is success-only). Lets a best-effort test prove terminate was attempted.
func (f *fakeCommander) terminateRequestIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.terminateReq...)
}

type broadcastCall struct {
	chatID string
	kind   string
}

// fakeBroadcaster is a thread-safe Broadcaster double.
type fakeBroadcaster struct {
	mu    sync.Mutex
	calls []broadcastCall
}

func (f *fakeBroadcaster) BroadcastAgentChat(chatID, kind string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, broadcastCall{chatID: chatID, kind: kind})
}

func (f *fakeBroadcaster) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

func (f *fakeBroadcaster) snapshot() []broadcastCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]broadcastCall{}, f.calls...)
}

type fakeWorkspace struct {
	home      string
	projectID string
	repoID    string
	worktree  string
	err       error
}

func (f *fakeWorkspace) WorktreeDir(
	_ context.Context,
	_ string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	if f.err != nil {
		return "", "", "", "", f.err
	}
	return f.home, f.projectID, f.repoID, f.worktree, nil
}

// fakeStore wraps a real agentchat.EventStore and lets a test force a chosen
// mutation/read to fail, exercising the usecase's error-wrap guard clauses
// without a fault-injecting database. A zero (nil) field delegates to the real
// store, so only the targeted call fails.
type fakeStore struct {
	agentchat.EventStore
	failGetChat   error
	failCreate    error
	failOpenSeg   error
	failEndSeg    error
	failListChats error
}

func (s *fakeStore) GetChat(ctx context.Context, id string) (domain.AgentChat, error) {
	if s.failGetChat != nil {
		return domain.AgentChat{}, s.failGetChat
	}
	return s.EventStore.GetChat(ctx, id)
}

func (s *fakeStore) Create(ctx context.Context, in agentchat.CreateInput) (domain.AgentChat, error) {
	if s.failCreate != nil {
		return domain.AgentChat{}, s.failCreate
	}
	return s.EventStore.Create(ctx, in)
}

func (s *fakeStore) OpenSegment(ctx context.Context, in agentchat.OpenSegmentInput) (domain.AgentChat, error) {
	if s.failOpenSeg != nil {
		return domain.AgentChat{}, s.failOpenSeg
	}
	return s.EventStore.OpenSegment(ctx, in)
}

func (s *fakeStore) EndSegment(ctx context.Context, chatID, segmentID string, now time.Time) (domain.AgentChat, error) {
	if s.failEndSeg != nil {
		return domain.AgentChat{}, s.failEndSeg
	}
	return s.EventStore.EndSegment(ctx, chatID, segmentID, now)
}

func (s *fakeStore) ListChats(ctx context.Context) ([]domain.AgentChat, error) {
	if s.failListChats != nil {
		return nil, s.failListChats
	}
	return s.EventStore.ListChats(ctx)
}

type testFixture struct {
	usecase *agentusecase.Usecase
	// repo is the REAL concrete EventStore, used for test reads; the usecase may
	// be built over a fault-injecting wrapper of it (see newFaultFixture) but
	// writes still land here.
	repo agentchat.EventStore
	// waitFn drains the asynx dispatch queue and runs every projection handler
	// (ax.WaitPublish), so a subsequent read observes all prior mutations with
	// no polling and no timeouts.
	waitFn func()
	term   *fakeCommander
	bc     *fakeBroadcaster
	ws     *fakeWorkspace
}

// wait blocks until every projection has folded.
func (f testFixture) wait() {
	f.waitFn()
}

// chat waits for quiescence then reads chatID from the read model.
func (f testFixture) chat(t *testing.T, chatID string) domain.AgentChat {
	t.Helper()
	f.wait()
	c, err := f.repo.GetChat(context.Background(), chatID)
	require.NoError(t, err)
	return c
}

// chatBySession waits then resolves the live chat bound to a provider session.
func (f testFixture) chatBySession(t *testing.T, sessionID string) domain.AgentChat {
	t.Helper()
	f.wait()
	c, err := f.repo.GetByProviderSession(context.Background(), sessionID)
	require.NoError(t, err)
	return c
}

// bcKinds waits for projection quiescence then returns the ordered lifecycle
// kinds of every hub frame captured so far. Blocking on WaitPublish (not a
// sleep) guarantees the async hub projection has folded every prior command.
func (f testFixture) bcKinds(t *testing.T) []string {
	t.Helper()
	f.wait()
	snap := f.bc.snapshot()
	kinds := make([]string, len(snap))
	for i, c := range snap {
		kinds[i] = c.kind
	}
	return kinds
}

// activeSegOf returns the single active segment for a crowbarSegID in a chat.
func activeSegOf(t *testing.T, chat domain.AgentChat, crowbarSegID string) domain.AgentSegment {
	t.Helper()
	for _, s := range chat.Segments {
		if s.CrowbarSegmentID == crowbarSegID && s.Status == "active" {
			return s
		}
	}
	t.Fatalf("no active segment for crowbarSegID %q in chat %+v", crowbarSegID, chat)
	return domain.AgentSegment{}
}

// segByID returns the segment with the given id from a chat.
func segByID(t *testing.T, chat domain.AgentChat, id string) domain.AgentSegment {
	t.Helper()
	for _, s := range chat.Segments {
		if s.ID == id {
			return s
		}
	}
	t.Fatalf("no segment %q in chat %+v", id, chat)
	return domain.AgentSegment{}
}

// newEventStore builds a throwaway in-memory asynx-backed EventStore wired to
// the given hub broadcast func and returns it alongside a wait closure
// (ax.WaitPublish) so tests can block on projection quiescence without importing
// the agentchat package's test-only helper (which is not visible across
// packages). broadcast is the SAME seam the production hub projection uses, so a
// test capturing frames through it exercises the real post-cutover lifecycle
// feed — the usecase no longer broadcasts on its own.
func newEventStore(
	t *testing.T,
	broadcast agentchat.BroadcastFunc,
) (agentchat.EventStore, func()) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentChat]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	repo, err := agentchat.NewEventSourced(ax, es, db, broadcast)
	require.NoError(t, err)
	return repo, ax.WaitPublish
}

func newFixture(t *testing.T) testFixture {
	t.Helper()
	f, _ := newFixtureUsing(t, nil)
	return f
}

// newFaultFixture builds a fixture whose usecase writes/reads through a
// fault-injecting wrapper the caller can arm; the fixture's repo stays the real
// underlying store for reads.
func newFaultFixture(t *testing.T) (testFixture, *fakeStore) {
	t.Helper()
	fs := &fakeStore{}
	f, _ := newFixtureUsing(t, func(real agentchat.EventStore) agentchat.EventStore {
		fs.EventStore = real
		return fs
	})
	return f, fs
}

// newFixtureUsing builds a fixture; wrap (if non-nil) adapts the real store into
// the store the usecase is built over.
func newFixtureUsing(
	t *testing.T,
	wrap func(agentchat.EventStore) agentchat.EventStore,
) (testFixture, agentchat.EventStore) {
	t.Helper()
	t.Setenv("CROWBAR_HOOK_BIN", "/fake/bin/crowbar")

	// bc captures frames through the EventStore's hub projection — the real
	// production lifecycle feed post-cutover — not through the usecase, which no
	// longer broadcasts.
	bc := &fakeBroadcaster{}
	real, waitFn := newEventStore(t, bc.BroadcastAgentChat)
	used := real
	if wrap != nil {
		used = wrap(real)
	}

	term := &fakeCommander{}
	ws := &fakeWorkspace{
		home:      t.TempDir(),
		projectID: "p1",
		repoID:    "r1",
		worktree:  t.TempDir(),
	}

	u := agentusecase.New(used, engineagent.NewRegistry(), term, ws)
	return testFixture{usecase: u, repo: real, waitFn: waitFn, term: term, bc: bc, ws: ws}, used
}

func TestSpawnChat_PersistsChatAndSegmentAndSpawns(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, segID)

	chat := f.chat(t, chatID)
	assert.Equal(t, "ws1", chat.WorkspaceID)
	assert.Equal(t, segID, chat.ActiveSegmentID)

	seg := activeSegOf(t, chat, segID)
	assert.Equal(t, "claude", seg.ProviderID)
	assert.Equal(t, segID, seg.CrowbarSegmentID)
	assert.Equal(t, "active", seg.Status)
	assert.NotEmpty(t, seg.TerminalSessionID)

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]
	assert.Equal(t, "ws1", call.workspaceID)
	assert.Equal(t, f.ws.worktree, call.cwd)
	assert.Equal(t, "claude", call.argv[0])
	// A fresh SpawnChat injects the title instruction via the descriptor's
	// system_prompt_inject mechanism; it must be present, not the raw ledger
	// handoff (there is none yet for a brand-new chat).
	assert.Contains(t, call.argv, "--append-system-prompt")
}

// TestSpawnSegment_TmpDirSurvivesSpawnAndIsRemovedOnlyWhenSessionEnds guards
// the resource-leak fix: spawnSegment's per-spawn tmp dir must still exist right
// after spawn and be removed only when the terminal engine invokes onExit.
func TestSpawnSegment_TmpDirSurvivesSpawnAndIsRemovedOnlyWhenSessionEnds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	tmpDir := filepath.Join(f.ws.home, "agent-tmp", segID)
	info, err := os.Stat(tmpDir)
	require.NoError(t, err, "agent-tmp dir must exist immediately after spawn")
	assert.True(t, info.IsDir())

	require.Equal(t, 1, f.term.callCount())
	require.NotNil(t, f.term.calls[0].onExit, "CreateCommand must receive a non-nil onExit")

	_, err = os.Stat(tmpDir)
	require.NoError(t, err, "agent-tmp dir must survive while the CLI is running")

	f.term.calls[0].onExit()

	_, err = os.Stat(tmpDir)
	assert.True(t, os.IsNotExist(err), "agent-tmp dir must be removed once the session ends")
}

func TestSpawnChat_UsesDescriptorCmdAsArgv0(t *testing.T) {
	for _, providerID := range []string{"claude", "codex"} {
		t.Run(providerID, func(t *testing.T) {
			f := newFixture(t)
			ctx := context.Background()

			_, segID, err := f.usecase.SpawnChat(ctx, "ws1", providerID)
			require.NoError(t, err)
			require.NotEmpty(t, segID)

			require.Equal(t, 1, f.term.callCount())
			assert.Equal(t, providerID, f.term.calls[0].argv[0])
		})
	}
}

func TestSpawnSegment_CrowbarHookPathFallsBackToHomeBinCrowbar(t *testing.T) {
	f := newFixture(t)
	// Override the fixture's default CROWBAR_HOOK_BIN so the path falls back to
	// <home>/bin/crowbar.
	t.Setenv("CROWBAR_HOOK_BIN", "")
	home := f.ws.home
	ctx := context.Background()

	_, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.Equal(t, 1, f.term.callCount())
	settingsPath := argAfter(t, f.term.calls[0].argv, "--settings")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), filepath.Join(home, "bin", "crowbar")+" hook")
}

func argAfter(t *testing.T, argv []string, flag string) string {
	t.Helper()
	for i, a := range argv {
		if a == flag && i+1 < len(argv) {
			return argv[i+1]
		}
	}
	t.Fatalf("flag %q not found in argv %v", flag, argv)
	return ""
}

// TestSpawn_HookConfigCarriesSegmentAndProvider guards the arg-based spawn
// attribution: the segment id and provider are rendered into the hook config
// command line, not injected as an env var.
func TestSpawn_HookConfigCarriesSegmentAndProvider(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.Equal(t, 1, f.term.callCount())
	call := f.term.calls[0]
	settingsPath := argAfter(t, call.argv, "--settings")
	data, err := os.ReadFile(settingsPath)
	require.NoError(t, err)
	assert.Contains(t, string(data), "--segment "+segID+" --provider claude")

	for _, kv := range call.env {
		assert.False(t, strings.HasPrefix(kv, "CROWBAR_SEGMENT_ID="), "env must not carry CROWBAR_SEGMENT_ID: %q", kv)
	}
}

func TestIngestHook_SessionStart_Bound_RecordsProviderSessionID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset() // drop the spawn's "created" frame; assert only the bind below

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-abc",
	}))
	require.NoError(t, err)

	seg := activeSegOf(t, f.chat(t, chatID), segID)
	assert.Equal(t, "sid-abc", seg.ProviderSessionID)

	// The BindSession command emits agentchat.session_bound → one hub frame.
	assert.Equal(t, []string{"session_bound"}, f.bcKinds(t))
}

// TestIngestHook_SessionStart_Bound_NeverOverwritesExistingProviderSessionID:
// the reducer only returns "bound" for a segment whose session it has not seen,
// so a session id ALREADY on the segment is a pre-existing binding the usecase
// must preserve (the guard in bindSession).
func TestIngestHook_SessionStart_Bound_NeverOverwritesExistingProviderSessionID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	_, err = f.repo.BindSession(ctx, chatID, segID, "sid-preexisting")
	require.NoError(t, err)
	f.wait()

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-new",
	}))
	require.NoError(t, err)

	seg := activeSegOf(t, f.chat(t, chatID), segID)
	assert.Equal(t, "sid-preexisting", seg.ProviderSessionID)
}

func TestIngestHook_SessionStart_SameSessionIsNoop(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset() // drop the spawn's "created" frame

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	// Only the first session_start binds (session_bound); the repeat of the same
	// session id is a reducer no-op that issues no command and so emits no frame.
	assert.Equal(t, []string{"session_bound"}, f.bcKinds(t))

	seg := activeSegOf(t, f.chat(t, chatID), segID)
	assert.Equal(t, "sid-1", seg.ProviderSessionID)
}

func TestIngestHook_SessionStart_Registered_MovesOldSegmentAndCreatesNewChat(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	// The original segment (id == segID) stays in the original chat, now ended.
	oldChat := f.chat(t, chatID)
	oldSeg := segByID(t, oldChat, segID)
	assert.Equal(t, "ended", oldSeg.Status)
	assert.Equal(t, "sid-1", oldSeg.ProviderSessionID)
	require.NotNil(t, oldSeg.EndedAt)

	// The live process moved to a brand-new chat bound to sid-2.
	newChat := f.chatBySession(t, "sid-2")
	newActive := activeSegOf(t, newChat, segID)
	assert.NotEqual(t, segID, newActive.ID)
	assert.Equal(t, "sid-2", newActive.ProviderSessionID)
	assert.NotEqual(t, chatID, newChat.ID)
	assert.Equal(t, oldSeg.TerminalSessionID, newActive.TerminalSessionID)
	assert.Equal(t, "ws1", newChat.WorkspaceID)
	assert.Equal(t, newActive.ID, newChat.ActiveSegmentID)
}

// TestIngestHook_SessionStart_Registered_ClearsVacatedChatsActiveSegmentID:
// once the live process moves away, the vacated chat's ActiveSegmentID must be
// cleared (EndSegment does this).
func TestIngestHook_SessionStart_Registered_ClearsVacatedChatsActiveSegmentID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	vacated := f.chat(t, chatID)
	assert.Empty(t, vacated.ActiveSegmentID, "vacated chat's ActiveSegmentID must be cleared")
}

func TestIngestHook_SessionStart_Focus_ReactivatesKnownChat(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	// Focused back into the original chat (sid-1): it now hosts the live process
	// again via a fresh active segment carrying the same crowbarSegID.
	chat := f.chat(t, chatID)
	active := activeSegOf(t, chat, segID)
	assert.Equal(t, "sid-1", active.ProviderSessionID)
	assert.Equal(t, active.ID, chat.ActiveSegmentID)
}

// TestIngestHook_SessionStart_Focus_ClearsVacatedChatsActiveSegmentID: the
// registered chat (sid-2's) loses its live segment when the process focuses
// back to sid-1's chat, so its ActiveSegmentID must be cleared too.
func TestIngestHook_SessionStart_Focus_ClearsVacatedChatsActiveSegmentID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	registeredChatID := f.chatBySession(t, "sid-2").ID

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	vacated := f.chat(t, registeredChatID)
	assert.Empty(t, vacated.ActiveSegmentID, "vacated chat's ActiveSegmentID must be cleared after focus moves away")
}

// TestIngestHook_TurnStopAppendsAssistantTurn: turn_stop appends the assistant
// message from the hook payload straight to the ledger.
func TestIngestHook_TurnStopAppendsAssistantTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset() // drop the spawn's "created" frame

	err = f.usecase.IngestHook(ctx, segID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "s1", "last_assistant_message": "done thing"}))
	require.NoError(t, err)

	handoff, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "assistant (claude): done thing")

	// turn_stop closes the turn (StopTurn → agentchat.turn_stopped); the ledger
	// append itself emits no aggregate event, so exactly one frame lands.
	assert.Equal(t, []string{"turn_stopped"}, f.bcKinds(t))
}

// TestIngestHook_UserPromptAppendsUserTurn: user_prompt appends a user turn,
// fires the derived-title fallback first (empty title), and opens the turn.
func TestIngestHook_UserPromptAppendsUserTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset() // drop the spawn's "created" frame

	err = f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "please do the thing"}))
	require.NoError(t, err)

	handoff, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "user: please do the thing")

	// The derived-title SetTitle (title_set) then StartTurn (turn_started) each
	// emit exactly one frame, in that order; the ledger append emits none.
	assert.Equal(t, []string{"title_set", "turn_started"}, f.bcKinds(t))
}

// TestIngestHook_TurnStop_EmptyMessage_AppendsNoLedgerTurnButStillClosesTurn: a
// turn_stop with no last_assistant_message writes no ledger entry (an empty
// message is a ledger no-op) but still closes the turn — StopTurn is a turn-state
// transition independent of ledger content, so exactly one turn_stopped frame
// lands and no ledger content accrues.
func TestIngestHook_TurnStop_EmptyMessage_AppendsNoLedgerTurnButStillClosesTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset() // drop the spawn's "created" frame

	err = f.usecase.IngestHook(ctx, segID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.NoError(t, err)

	assert.Equal(t, []string{"turn_stopped"}, f.bcKinds(t))

	handoff, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, handoff, "an empty assistant message must append no ledger turn")
}

func TestIngestHook_TurnStop_AfterMove_AttributesToNewChat(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))
	f.wait()

	newChatID := f.chatBySession(t, "sid-2").ID

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "turn_stop", mustJSON(t, map[string]any{
		"session_id":             "sid-2",
		"last_assistant_message": "second chat transcript",
	})))

	handoff, err := f.usecase.AssembleHandoff(ctx, newChatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "second chat transcript")
}

func TestIngestHook_UnknownSegment_IsIgnored(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.IngestHook(ctx, "does-not-exist", "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.NoError(t, err)
	assert.Empty(t, f.bc.snapshot())
}

func TestIngestHook_UnmappedCanonicalEvent_ReturnsNil(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	f.bc.reset() // drop the spawn's "created" frame

	err = f.usecase.IngestHook(ctx, segID, "claude", "not_a_real_hook", mustJSON(t, map[string]any{}))
	require.NoError(t, err)
	assert.Empty(t, f.bcKinds(t), "an unmapped hook issues no command and so emits no frame")
}

func TestSeedRegistry_RehydratesKnownSessions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	// Seed a persisted chat cK with two ended segments: one unbound (no session,
	// the seed loop skips it) and one bound to "sid-known".
	_, err := f.repo.Create(ctx, agentchat.CreateInput{
		ID: "cK", WorkspaceID: "ws1", SegmentID: "sK1", CrowbarSegmentID: "csK1", ProviderID: "claude", TerminalSession: "term-x",
	})
	require.NoError(t, err)
	_, err = f.repo.EndSegment(ctx, "cK", "sK1", timeUnix(1))
	require.NoError(t, err)
	_, err = f.repo.OpenSegment(ctx, agentchat.OpenSegmentInput{
		ChatID: "cK", SegmentID: "sK2", CrowbarSegmentID: "csK2", ProviderID: "claude", TerminalSession: "term-y", Now: timeUnix(2),
	})
	require.NoError(t, err)
	_, err = f.repo.BindSession(ctx, "cK", "csK2", "sid-known")
	require.NoError(t, err)
	_, err = f.repo.EndSegment(ctx, "cK", "sK2", timeUnix(3))
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.SeedRegistry(ctx))

	_, segA, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segA, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "s0"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segA, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-known"})))

	// A known session id (sid-known → cK) makes the move a "focus" into cK, not a
	// "registered" new chat: cK now hosts the live process (crowbarSegID segA).
	cK := f.chat(t, "cK")
	active := activeSegOf(t, cK, segA)
	assert.Equal(t, "sid-known", active.ProviderSessionID)
	assert.Equal(t, active.ID, cK.ActiveSegmentID)
}

func TestListChatsGetChatSegmentsFor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	chats, err := f.usecase.ListChats(ctx)
	require.NoError(t, err)
	require.Len(t, chats, 1)
	assert.Equal(t, chatID, chats[0].ID)

	chat, err := f.usecase.GetChat(ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, segID, chat.ActiveSegmentID)

	segs, err := f.usecase.SegmentsFor(ctx, chatID)
	require.NoError(t, err)
	require.Len(t, segs, 1)
	assert.Equal(t, segID, segs[0].ID)
}

// TestIngestHook_UserPromptOpensTurn_TurnStopClosesTurn pins Fix 1: the
// user_prompt and turn_stop hooks drive the aggregate's live turn state through
// StartTurn/StopTurn, so domain.AgentChat.Working and CurrentTurnStarted reflect
// reality (the durable working-state the boot reconcile repairs) instead of
// being perpetually false as they were when StartTurn/StopTurn had no callers.
func TestIngestHook_UserPromptOpensTurn_TurnStopClosesTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()
	require.False(t, f.chat(t, chatID).Working, "a fresh chat is not Working")

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "start working"})))
	working := f.chat(t, chatID)
	assert.True(t, working.Working, "a user_prompt must open the turn (Working==true)")
	require.NotNil(t, working.CurrentTurnStarted, "an open turn must record CurrentTurnStarted")

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "done"})))
	idle := f.chat(t, chatID)
	assert.False(t, idle.Working, "a turn_stop must close the turn (Working==false)")
	assert.Nil(t, idle.CurrentTurnStarted, "a closed turn must clear CurrentTurnStarted")
}

// TestBroadcast_OneFramePerLifecycleEvent pins Fix 2's single-broadcaster
// invariant: a full chat lifecycle produces exactly one hub frame per aggregate
// event, in order, with the consolidated kind vocabulary — no duplicate or
// divergent frames from a now-deleted second (usecase) broadcaster. Each f.wait
// between steps folds that step's events before the next reads the aggregate.
func TestBroadcast_OneFramePerLifecycleEvent(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "hello"})))
	f.wait()
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": "hi"})))
	f.wait()
	_, err = f.usecase.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	f.wait()
	require.NoError(t, f.usecase.DeleteChat(ctx, chatID))

	assert.Equal(t, []string{
		"created",        // SpawnChat
		"title_set",      // user_prompt: derived title
		"turn_started",   // user_prompt: StartTurn
		"turn_stopped",   // turn_stop: StopTurn
		"segment_ended",  // switch: end outgoing segment
		"segment_opened", // switch: open incoming segment
		"deleted",        // DeleteChat
	}, f.bcKinds(t))
}

// --- error paths ------------------------------------------------------------

func TestSpawnChat_CreatePersistFailure_TearsDownCLIAndWraps(t *testing.T) {
	f, fs := newFaultFixture(t)
	fs.failCreate = fmt.Errorf("boom: create")
	ctx := context.Background()

	_, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "persist segment")

	// The CLI was already spawned (term-1) and must be torn down so no orphan
	// process leaks when the persist fails.
	require.Equal(t, 1, f.term.callCount())
	assert.Contains(t, f.term.terminatedIDs(), "term-1")
}

func TestSpawnChat_CreateCommandFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	f.term.err = fmt.Errorf("boom: create command")
	ctx := context.Background()

	_, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "create command")
}

func TestSpawnChat_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	f.ws.err = fmt.Errorf("boom: worktree lookup")
	ctx := context.Background()

	_, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree dir")
}

func TestSpawnChat_UnknownProvider_ReturnsWrappedDescriptorError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, _, err := f.usecase.SpawnChat(ctx, "ws1", "not-a-real-provider")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve descriptor")
}

func TestIngestHook_ChatLookupFailure_ReturnsWrappedError(t *testing.T) {
	f, fs := newFaultFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	fs.failGetChat = fmt.Errorf("boom: get chat")
	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ingest hook: chat")
}

func TestIngestHook_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	f.wait()

	f.ws.err = fmt.Errorf("boom: worktree lookup")
	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree dir")
}

func TestSeedRegistry_ListChatsFailure_ReturnsWrappedError(t *testing.T) {
	f, fs := newFaultFixture(t)
	fs.failListChats = fmt.Errorf("boom: list chats")
	ctx := context.Background()

	err := f.usecase.SeedRegistry(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "seed registry")
}

// timeUnix is a tiny helper for deterministic timestamps in seed setup.
func timeUnix(sec int64) time.Time { return time.Unix(sec, 0).UTC() }
