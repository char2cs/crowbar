package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
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
// the spawn and hands back a unique "term-N" session id; TerminateGraceful records
// the id. The mutex makes it safe under -race.
//
// It deliberately does NOT fire onExit when a session is terminated. A real SIGTERM
// does not kill a process synchronously either — the engine's reap goroutine calls
// onExit whenever the CLI actually dies — so a test that wants the death observed
// calls f.term.exit(id) explicitly, which is also the only honest way to express
// "and THEN the process died".
//
// deadSessions backs SessionLive (the PTY liveness authority): every session is
// alive unless killed.
type fakeCommander struct {
	mu           sync.Mutex
	calls        []commandCall
	terminated   []string
	terminateReq []string
	byID         map[string]int // terminal session id -> index into calls
	nextID       int
	err          error
	terminateErr error
	deadSessions map[string]bool
	// duringFork runs INSIDE CreateCommand, before the session id exists — the window a
	// real spawn holds open while the OS forks a process. It is how a test drives
	// something that genuinely happens concurrently with a spawn (a hook, which is never
	// gated) without any timing: the interleaving is exact, not hoped for.
	//
	// It is invoked WITHOUT the mutex held, so whatever it drives may call back into this
	// fake (a hook that retires a runner terminates its PTY).
	duringFork func()
	// duringTerminate runs INSIDE TerminateGraceful, before the kill is recorded — the
	// moment the outgoing CLI is asked to die. It is how the mid-turn tests OBSERVE that
	// moment as it happens (a switch that kills a CLI mid-answer is the bug), rather than
	// inspecting the record afterwards and having to reason about when it was written.
	//
	// Invoked WITHOUT the mutex held, like duringFork.
	duringTerminate func(sessionID string)
}

func (f *fakeCommander) CreateCommand(
	_ context.Context,
	workspaceID string,
	cwd string,
	argv []string,
	env []string,
	onExit func(),
) (string, error) {
	if f.duringFork != nil {
		f.duringFork()
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return "", f.err
	}
	f.nextID++
	id := fmt.Sprintf("term-%d", f.nextID)
	if f.byID == nil {
		f.byID = map[string]int{}
	}
	f.byID[id] = len(f.calls)
	f.calls = append(f.calls, commandCall{
		workspaceID: workspaceID,
		cwd:         cwd,
		argv:        append([]string{}, argv...),
		env:         append([]string{}, env...),
		onExit:      onExit,
	})
	return id, nil
}

func (f *fakeCommander) TerminateGraceful(
	_ context.Context,
	sessionID string,
) error {
	if f.duringTerminate != nil {
		f.duringTerminate(sessionID)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	// terminateReq records EVERY attempt (even failed ones), so a best-effort caller
	// can assert the terminate was attempted regardless of outcome; terminated stays
	// success-only for callers that only care about the ids actually torn down.
	f.terminateReq = append(f.terminateReq, sessionID)
	if f.terminateErr != nil {
		return f.terminateErr
	}
	f.terminated = append(f.terminated, sessionID)
	return nil
}

// SessionLive implements the agent.TerminalCommander liveness seam: a session's PTY
// is alive unless its process has exited.
func (f *fakeCommander) SessionLive(_ context.Context, sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.deadSessions[sessionID]
}

// exit models the PTY actually dying: the session stops being live and the terminal
// engine invokes the onExit callback it was handed at spawn — which is the ONE event
// that makes a runner dead. Tests call it to say "and then the CLI died", whether
// that death followed a TerminateGraceful or was the CLI exiting on its own.
func (f *fakeCommander) exit(t *testing.T, sessionID string) {
	t.Helper()
	f.mu.Lock()
	if f.deadSessions == nil {
		f.deadSessions = map[string]bool{}
	}
	f.deadSessions[sessionID] = true
	idx, ok := f.byID[sessionID]
	// Copy the callback out UNDER the lock: calls is appended to by CreateCommand on
	// another goroutine, so reading it after unlocking is a data race (the backing
	// array can be reallocated mid-read).
	var onExit func()
	if ok {
		onExit = f.calls[idx].onExit
	}
	f.mu.Unlock()
	require.True(t, ok, "no spawned terminal session %q", sessionID)
	onExit()
}

// dieWithDaemon models a DAEMON RESTART: every PTY dies, and NO onExit callback ever
// fires. That second half is the whole point, and it is what makes a restart different
// from every other death in these tests — the callback lives in the process that just
// went away, so nothing records the deaths. What survives is a durable sqlite table full
// of live-runner rows describing CLIs that no longer exist, which is exactly the state
// boot reconciliation is handed.
func (f *fakeCommander) dieWithDaemon() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.deadSessions == nil {
		f.deadSessions = map[string]bool{}
	}
	for id := range f.byID {
		f.deadSessions[id] = true
	}
}

func (f *fakeCommander) callCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.calls)
}

// terminatedIDs returns every session id TerminateGraceful successfully tore down.
func (f *fakeCommander) terminatedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.terminated...)
}

// terminateRequestIDs returns every session id TerminateGraceful was CALLED with,
// including attempts that returned an error (unlike terminatedIDs, which is
// success-only). Lets a best-effort test prove terminate was attempted.
func (f *fakeCommander) terminateRequestIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.terminateReq...)
}

type broadcastCall struct {
	chatID      string
	workspaceID string
	kind        string
	// working is the aggregate's folded busy state as of the frame — what the FE's
	// spinner reads. Captured so a test can assert the SPINNER, not just the kind.
	working bool
}

// fakeBroadcaster is a thread-safe Broadcaster double for agentchat frames.
type fakeBroadcaster struct {
	mu    sync.Mutex
	calls []broadcastCall
}

func (f *fakeBroadcaster) BroadcastAgentChat(chatID, workspaceID, kind string, working bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, broadcastCall{
		chatID:      chatID,
		workspaceID: workspaceID,
		kind:        kind,
		working:     working,
	})
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

type runnerFrame struct {
	runnerID    string
	workspaceID string
	chatID      string
	kind        string
}

// fakeRunnerBroadcaster captures the agentrunner hub frames (started/session_bound/
// moved/exited) the runner projections emit.
type fakeRunnerBroadcaster struct {
	mu     sync.Mutex
	frames []runnerFrame
}

func (f *fakeRunnerBroadcaster) BroadcastAgentRunner(runnerID, workspaceID, chatID, kind string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames = append(f.frames, runnerFrame{runnerID: runnerID, workspaceID: workspaceID, chatID: chatID, kind: kind})
}

func (f *fakeRunnerBroadcaster) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frames = nil
}

func (f *fakeRunnerBroadcaster) snapshot() []runnerFrame {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]runnerFrame{}, f.frames...)
}

type fakeWorkspace struct {
	home      string
	projectID string
	repoID    string
	worktree  string
	chatsDir  string
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

func (f *fakeWorkspace) AgentChatsDir(
	_ context.Context,
	_ string,
) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.chatsDir, nil
}

// fakeChatStore wraps a real agentchat.EventStore and lets a test force a chosen
// mutation/read to fail, exercising the usecase's error-wrap guard clauses without
// a fault-injecting database. A nil field delegates to the real store, so only the
// targeted call fails.
type fakeChatStore struct {
	agentchat.EventStore
	failGetChat   error
	failCreate    error
	failListChats error
}

func (s *fakeChatStore) GetChat(ctx context.Context, id string) (domain.AgentChat, error) {
	if s.failGetChat != nil {
		return domain.AgentChat{}, s.failGetChat
	}
	return s.EventStore.GetChat(ctx, id)
}

func (s *fakeChatStore) Create(ctx context.Context, in agentchat.CreateInput) (domain.AgentChat, error) {
	if s.failCreate != nil {
		return domain.AgentChat{}, s.failCreate
	}
	return s.EventStore.Create(ctx, in)
}

func (s *fakeChatStore) ListChats(ctx context.Context) ([]domain.AgentChat, error) {
	if s.failListChats != nil {
		return nil, s.failListChats
	}
	return s.EventStore.ListChats(ctx)
}

// fakeRunnerStore is the same fault-injecting wrapper for the runner aggregate.
type fakeRunnerStore struct {
	agentrunner.EventStore
	failStart      error
	failMove       error
	failForgetChat error
	// afterMove / afterStart run once each COMMAND HAS COMMITTED, so a test can pin an
	// exact interleaving of two placements onto one chat by blocking on channels inside
	// them. Real signals, never a sleep: the goroutines hand off to each other.
	afterMove  func()
	afterStart func()
}

func (s *fakeRunnerStore) ForgetChat(ctx context.Context, chatID string) error {
	if s.failForgetChat != nil {
		return s.failForgetChat
	}
	return s.EventStore.ForgetChat(ctx, chatID)
}

func (s *fakeRunnerStore) Start(ctx context.Context, in agentrunner.StartInput) (domain.AgentRunner, error) {
	if s.failStart != nil {
		return domain.AgentRunner{}, s.failStart
	}
	r, err := s.EventStore.Start(ctx, in)
	if s.afterStart != nil {
		s.afterStart()
	}
	return r, err
}

func (s *fakeRunnerStore) Move(
	ctx context.Context,
	runnerID, toChatID, sessionID string,
	now time.Time,
) (domain.AgentRunner, error) {
	if s.failMove != nil {
		return domain.AgentRunner{}, s.failMove
	}
	r, err := s.EventStore.Move(ctx, runnerID, toChatID, sessionID, now)
	if s.afterMove != nil {
		s.afterMove()
	}
	return r, err
}

// testFixture is the usecase harness: the real asynx-backed chat AND runner
// aggregates (in-memory), with the terminal engine, the workspace reader and both
// hub feeds faked.
type testFixture struct {
	ctx     context.Context
	usecase *agentusecase.Usecase
	// chats/runners are the REAL concrete EventStores, used for test reads; the
	// usecase may be built over a fault-injecting wrapper of them (newFaultFixture)
	// but writes still land here.
	chats   agentchat.EventStore
	runners agentrunner.EventStore
	// waitFn drains both asynx dispatch queues and runs every projection handler
	// (ax.WaitPublish), so a subsequent read observes all prior mutations with no
	// polling and no timeouts.
	waitFn   func()
	term     *fakeCommander
	bc       *fakeBroadcaster
	rbc      *fakeRunnerBroadcaster
	ws       *fakeWorkspace
	registry *engineagent.Registry
}

// wait blocks until every projection has folded.
func (f testFixture) wait() {
	f.waitFn()
}

// spawn creates a chat with a live runner on it, and blocks until both aggregates'
// projections have folded — so the very next call reads a settled model.
func (f testFixture) spawn(t *testing.T, provider string) (chatID, runnerID string) {
	t.Helper()
	chatID, runnerID, err := f.usecase.SpawnChat(f.ctx, "ws1", provider)
	require.NoError(t, err)
	f.wait()
	return chatID, runnerID
}

// announce drives a session_start hook: the vendor CLI reporting which conversation
// it is now in. This is the ONLY way a conversation change ever reaches Crowbar —
// nothing inspects terminal input — so every /clear, /new and /resume in these tests
// is expressed exactly as the real CLI expresses it.
func (f testFixture) announce(t *testing.T, runnerID, sessionID string) {
	t.Helper()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "", "session_start",
		mustJSON(t, map[string]any{"session_id": sessionID})))
	f.wait()
}

// turn drives a turn_stop hook: the CLI finishing a turn, which is how a line ever
// gets into a chat's ledger in production. It is the ONLY way these tests write
// ledger content — no test reaches around the hook path to plant a turn — because the
// ledger's provider tag and timestamps are exactly what the resume path later reads.
func turn(t *testing.T, f testFixture, runnerID, provider, content string) {
	t.Helper()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, provider, "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": content})))
	f.wait()
}

// chat waits for quiescence then reads chatID from the read model.
func (f testFixture) chat(t *testing.T, chatID string) domain.AgentChat {
	t.Helper()
	f.wait()
	c, err := f.chats.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	return c
}

// runner reads a live runner from the read model.
func (f testFixture) runner(t *testing.T, runnerID string) domain.AgentRunner {
	t.Helper()
	f.wait()
	r, err := f.runners.Get(f.ctx, runnerID)
	require.NoError(t, err)
	return r
}

// liveRunnerFor answers "is this chat live, and who is on it" — the query that
// replaced ActiveSegmentID.
func (f testFixture) liveRunnerFor(t *testing.T, chatID string) (domain.AgentRunner, error) {
	t.Helper()
	f.wait()
	return f.runners.LiveRunnerForChat(f.ctx, chatID)
}

// placedRunnersFor returns every live runner POINTED AT chatID. LiveRunnerForChat only
// answers "who holds it", so this is the invariant check: I2 says the answer must never
// be more than one, at any instant. A runner Crowbar has taken off a chat (Displace) is
// no longer pointed anywhere and so is not counted, even while its process is still
// dying.
func (f testFixture) placedRunnersFor(t *testing.T, chatID string) []domain.AgentRunner {
	t.Helper()
	f.wait()
	all, err := f.runners.AllLive(f.ctx)
	require.NoError(t, err)
	var out []domain.AgentRunner
	for _, r := range all {
		if r.CurrentChatID == chatID {
			out = append(out, r)
		}
	}
	return out
}

// chatForSession resolves which chat hosts a conversation, from append-only history.
func (f testFixture) chatForSession(t *testing.T, sessionID string) string {
	t.Helper()
	f.wait()
	chatID, err := f.runners.ChatForSession(f.ctx, "ws1", sessionID)
	require.NoError(t, err)
	return chatID
}

// bcKinds waits for projection quiescence then returns the ordered lifecycle kinds
// of every CHAT hub frame captured so far. Blocking on WaitPublish (not a sleep)
// guarantees the async hub projection has folded every prior command.
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

// runnerKinds is the same for the RUNNER hub frames.
func (f testFixture) runnerKinds(t *testing.T) []string {
	t.Helper()
	f.wait()
	snap := f.rbc.snapshot()
	kinds := make([]string, len(snap))
	for i, fr := range snap {
		kinds[i] = fr.kind
	}
	return kinds
}

// newChatStore builds a throwaway in-memory asynx-backed agentchat EventStore wired
// to the given hub broadcast func, returning it with its WaitPublish. broadcast is
// the SAME seam the production hub projection uses, so a test capturing frames
// through it exercises the real lifecycle feed — the usecase never broadcasts itself.
func newChatStore(
	t *testing.T,
	broadcast agentchat.BroadcastFunc,
) (agentchat.EventStore, func()) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentChat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
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

// newRunnerStore builds the same for the agentrunner aggregate: the real commands,
// the real live-runner + conversation-history projections, the real hub projection.
func newRunnerStore(
	t *testing.T,
	broadcast agentrunner.BroadcastFunc,
) (agentrunner.EventStore, func()) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.AgentRunner]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	repo, err := agentrunner.NewEventSourced(ax, es, db, broadcast)
	require.NoError(t, err)
	return repo, ax.WaitPublish
}

func newFixture(t *testing.T) testFixture {
	t.Helper()
	f, _, _ := newFixtureUsing(t, nil, nil)
	return f
}

// newFaultFixture builds a fixture whose usecase writes/reads through fault-injecting
// wrappers the caller can arm; the fixture's own stores stay the real ones for reads.
func newFaultFixture(t *testing.T) (testFixture, *fakeChatStore, *fakeRunnerStore) {
	t.Helper()
	cs := &fakeChatStore{}
	rs := &fakeRunnerStore{}
	f, _, _ := newFixtureUsing(t,
		func(real agentchat.EventStore) agentchat.EventStore { cs.EventStore = real; return cs },
		func(real agentrunner.EventStore) agentrunner.EventStore { rs.EventStore = real; return rs },
	)
	return f, cs, rs
}

// newFixtureUsing builds a fixture; wrapChats/wrapRunners (if non-nil) adapt the real
// stores into the stores the usecase is built over.
func newFixtureUsing(
	t *testing.T,
	wrapChats func(agentchat.EventStore) agentchat.EventStore,
	wrapRunners func(agentrunner.EventStore) agentrunner.EventStore,
) (testFixture, agentchat.EventStore, agentrunner.EventStore) {
	t.Helper()
	t.Setenv("CROWBAR_HOOK_BIN", "/fake/bin/crowbar")

	bc := &fakeBroadcaster{}
	rbc := &fakeRunnerBroadcaster{}
	realChats, waitChats := newChatStore(t, bc.BroadcastAgentChat)
	realRunners, waitRunners := newRunnerStore(t, rbc.BroadcastAgentRunner)

	usedChats := realChats
	if wrapChats != nil {
		usedChats = wrapChats(realChats)
	}
	usedRunners := realRunners
	if wrapRunners != nil {
		usedRunners = wrapRunners(realRunners)
	}

	term := &fakeCommander{}
	// The default fixture models a MANAGED worktree ROOTED UNDER crowbar home:
	// worktree = <home>/projects/p1/slug/branch/worktree, so its sibling chats dir
	// (worktreepath.ChatsDir) is <home>/projects/p1/slug/branch/chats — strictly under
	// home. This mirrors production and is load-bearing now that every agent-path
	// removal is guarded by RemoveUnderHome (a chats dir NOT under home is refused).
	home := t.TempDir()
	worktree := filepath.Join(home, "projects", "p1", "slug", "branch", "worktree")
	ws := &fakeWorkspace{
		home:      home,
		projectID: "p1",
		repoID:    "r1",
		worktree:  worktree,
		chatsDir:  worktreepath.ChatsDir(worktree),
	}

	reg := engineagent.NewRegistry()
	u := agentusecase.New(usedChats, usedRunners, reg, term, ws)
	f := testFixture{
		ctx:      context.Background(),
		usecase:  u,
		chats:    realChats,
		runners:  realRunners,
		waitFn:   func() { waitChats(); waitRunners() },
		term:     term,
		bc:       bc,
		rbc:      rbc,
		ws:       ws,
		registry: reg,
	}
	return f, realChats, realRunners
}

// indexOf is the argv helper the spawn/switch tests assert with.
func indexOf(ss []string, target string) int {
	for i, s := range ss {
		if s == target {
			return i
		}
	}
	return -1
}

// runnersMove drives a raw runner Move — used only to CONSTRUCT an already-broken invariant
// (two runners placed on one chat), which the usecase must then heal. No production path
// reaches this directly.
func (f testFixture) runnersMove(t *testing.T, runnerID, chatID, sessionID string) error {
	t.Helper()
	_, err := f.runners.Move(f.ctx, runnerID, chatID, sessionID, time.Now())
	f.wait()
	return err
}
