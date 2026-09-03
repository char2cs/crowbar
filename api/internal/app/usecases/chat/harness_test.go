package chat_test

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
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
)

// mustJSON encodes one vendor hook payload. Every payload in this package is a hook
// payload, so it stamps the field every real hook of a real conversation carries and
// no test should have to remember: the transcript it belongs to.
//
// That is not decoration. Verified against codex 0.146.0, transcript_path is present
// on a fresh start, on a resume, and on every turn hook in between — and NULL on the
// internal session codex runs to write its memories, which is the only thing
// separating that session from the user typing /new (see the codex descriptor's
// require_payload_fields, and TestOwnsConversation_CodexInternalMemorySession). A
// payload with no transcript is therefore a MEANINGFUL payload here, not a shorthand,
// and a test that wants one says so by setting transcript_path to nil itself.
func mustJSON(t *testing.T, m map[string]any) []byte {
	t.Helper()
	if _, set := m["transcript_path"]; !set {
		m["transcript_path"] = "/rollouts/transcript.jsonl"
	}
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
	// duringForkCall is the argument-aware twin used when a test needs the runner
	// id embedded in the rendered hook command. It runs in the same exact window.
	duringForkCall func(commandCall)
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
	if f.duringForkCall != nil {
		f.duringForkCall(commandCall{
			workspaceID: workspaceID,
			cwd:         cwd,
			argv:        append([]string{}, argv...),
			env:         append([]string{}, env...),
			onExit:      onExit,
		})
	}
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

// SessionLive implements the agentusecase.TerminalCommander liveness seam: a session's PTY
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

func (f *fakeBroadcaster) BroadcastAgentChatFolder(_, _, _ string) {}

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

// watchAgentChat adapts the repository's announcement seam onto this fake's existing
// frame recorder, so every assertion in this package keeps its current shape.
func (f *fakeBroadcaster) watchAgentChat(e agentchat.ChatEvent) {
	f.BroadcastAgentChat(e.ChatID, e.WorkspaceID, e.Kind, e.Working && !e.Forgotten)
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

// watchAgentRunner adapts the repository's announcement seam onto this fake's
// existing frame recorder, so every assertion here keeps its current shape.
func (f *fakeRunnerBroadcaster) watchAgentRunner(e agentrunner.RunnerEvent) {
	f.BroadcastAgentRunner(e.RunnerID, e.WorkspaceID, e.ChatID, e.Kind)
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
//
// It also RECORDS the chat ids AbandonTurn was called with. Recording, not faulting, is
// what that one needs: closeAbandonedTurn is best-effort and swallows every error it gets,
// so a fault there would be invisible to the test — the only observable fact is whether
// the call happened at all, and for which id.
type fakeChatStore struct {
	agentchat.EventStore
	failGetChat   error
	failCreate    error
	failListChats error
	// failSetSelection / failLoadChat arm the two writes-and-reads the model and
	// effort selection travels through, so the "a spawn whose selection cannot be
	// read must fail before it forks" paths are reachable from a test.
	failSetSelection error
	failLoadChat     error
	// failLoadChatAfter lets that failure land on the Nth fold rather than the
	// first: a spawn folds the chat twice — once for its lineage, once for its
	// selection — so failing every fold can only ever prove the first.
	failLoadChatAfter int
	// staleProjection makes GetChat — the READ-MODEL read — answer with the
	// placement every chat had before it was placed anywhere, while LoadChat (the
	// event-log fold, reached through the embedded store) keeps answering
	// correctly.
	//
	// It models the real daemon's ordinary state rather than a broken one:
	// SetPlacement is deliberately on the async Send path, so between that write
	// returning and its projection folding, the read model genuinely serves the old
	// parent. Live, that window is microseconds wide and a test racing it passes
	// half the time — which is how a spawn that decided on projected state, and
	// therefore threaded nothing, survived a suite. Forcing the window open turns
	// the property into one a test can actually hold: whatever the projection says,
	// a decision about placement must not come from it.
	staleProjection bool

	// onStopTurn runs INSIDE StopTurn, before the aggregate is written. It is the
	// only seam that can observe what a client would see the instant the turn-state
	// change is published, which is what the assistant-message ordering turns on.
	onStopTurn func()

	mu           sync.Mutex
	abandonedIDs []string
}

func (s *fakeChatStore) StopTurn(
	ctx context.Context,
	chatID string,
	now time.Time,
	asyncWork int,
) (domain.Chat, error) {
	if s.onStopTurn != nil {
		s.onStopTurn()
	}
	return s.EventStore.StopTurn(ctx, chatID, now, asyncWork)
}

func (s *fakeChatStore) AbandonTurn(
	ctx context.Context,
	chatID string,
	now time.Time,
) (domain.Chat, error) {
	s.mu.Lock()
	s.abandonedIDs = append(s.abandonedIDs, chatID)
	s.mu.Unlock()
	return s.EventStore.AbandonTurn(ctx, chatID, now)
}

// abandonTurnIDs returns every chat id AbandonTurn has been asked to close.
func (s *fakeChatStore) abandonTurnIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.abandonedIDs...)
}

// forget drops what has been recorded so far, so a test can assert on ONE step of a
// scenario without the setup's own calls counting against it.
func (s *fakeChatStore) forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.abandonedIDs = nil
}

func (s *fakeChatStore) SetSelection(
	ctx context.Context,
	chatID, model, effort string,
) (domain.Chat, error) {
	if s.failSetSelection != nil {
		return domain.Chat{}, s.failSetSelection
	}
	return s.EventStore.SetSelection(ctx, chatID, model, effort)
}

func (s *fakeChatStore) LoadChat(ctx context.Context, id string) (domain.Chat, error) {
	if s.failLoadChat != nil {
		if s.failLoadChatAfter <= 0 {
			return domain.Chat{}, s.failLoadChat
		}
		s.failLoadChatAfter--
	}
	return s.EventStore.LoadChat(ctx, id)
}

func (s *fakeChatStore) GetChat(ctx context.Context, id string) (domain.Chat, error) {
	if s.failGetChat != nil {
		return domain.Chat{}, s.failGetChat
	}
	chat, err := s.EventStore.GetChat(ctx, id)
	if err != nil || !s.staleProjection {
		return chat, err
	}
	chat.ParentID = ""
	return chat, nil
}

func (s *fakeChatStore) Create(ctx context.Context, in agentchat.CreateInput) (domain.Chat, error) {
	if s.failCreate != nil {
		return domain.Chat{}, s.failCreate
	}
	return s.EventStore.Create(ctx, in)
}

func (s *fakeChatStore) ListChats(ctx context.Context) ([]domain.Chat, error) {
	if s.failListChats != nil {
		return nil, s.failListChats
	}
	return s.EventStore.ListChats(ctx)
}

// fakeRunnerStore is the same fault-injecting wrapper for the runner aggregate, and
// records the chat ids LiveRunnerForChat was asked about (see fakeChatStore for why
// recording rather than faulting).
type fakeRunnerStore struct {
	agentrunner.EventStore
	failStart      error
	failGet        error
	failGetAfter   int
	afterGet       func(engineagents.Runner)
	failMove       error
	failForgetChat error
	// afterMove / afterStart run once each COMMAND HAS COMMITTED, so a test can pin an
	// exact interleaving of two placements onto one chat by blocking on channels inside
	// them. Real signals, never a sleep: the goroutines hand off to each other.
	afterMove  func()
	afterStart func()

	mu         sync.Mutex
	lookedUpAt []string
}

func (s *fakeRunnerStore) Get(
	ctx context.Context,
	runnerID string,
) (engineagents.Runner, error) {
	if s.failGet != nil {
		if s.failGetAfter > 0 {
			s.failGetAfter--
			return s.EventStore.Get(ctx, runnerID)
		}
		return engineagents.Runner{}, s.failGet
	}
	runner, err := s.EventStore.Get(ctx, runnerID)
	if err == nil && s.afterGet != nil {
		s.afterGet(runner)
	}
	return runner, err
}

func (s *fakeRunnerStore) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (engineagents.Runner, error) {
	s.mu.Lock()
	s.lookedUpAt = append(s.lookedUpAt, chatID)
	s.mu.Unlock()
	return s.EventStore.LiveRunnerForChat(ctx, chatID)
}

// liveRunnerForChatIDs returns every chat id the live-runner query has been asked about.
func (s *fakeRunnerStore) liveRunnerForChatIDs() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string{}, s.lookedUpAt...)
}

// forget drops what has been recorded so far — see fakeChatStore.forget.
func (s *fakeRunnerStore) forget() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookedUpAt = nil
}

func (s *fakeRunnerStore) ForgetChat(ctx context.Context, chatID string) error {
	if s.failForgetChat != nil {
		return s.failForgetChat
	}
	return s.EventStore.ForgetChat(ctx, chatID)
}

func (s *fakeRunnerStore) Start(ctx context.Context, in agentrunner.StartInput) (engineagents.Runner, error) {
	if s.failStart != nil {
		return engineagents.Runner{}, s.failStart
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
	resumable bool,
	now time.Time,
) (engineagents.Runner, error) {
	if s.failMove != nil {
		return engineagents.Runner{}, s.failMove
	}
	r, err := s.EventStore.Move(ctx, runnerID, toChatID, sessionID, resumable, now)
	if s.afterMove != nil {
		s.afterMove()
	}
	return r, err
}

// harnessUsecase drives a whole chat — spawn, hook, read, answer — through the
// FIVE PORTS rather than through the concrete usecase behind them.
//
// One *Usecase satisfies all five, so this could just be that value. It is not,
// deliberately: embedding the ports means every call a test makes has to be
// reachable through the port that declares it, so a method quietly dropped from a
// port fails here instead of only at the route that needed it.
type harnessUsecase struct {
	agentusecase.ChatUsecase
	agentusecase.TurnUsecase
	agentusecase.RunnerUsecase
	agentusecase.AnswerUsecase
	agentusecase.ProviderUsecase
}

// testFixture is the usecase harness: the real asynx-backed chat AND runner
// aggregates (in-memory), with the terminal engine, the workspace reader and both
// hub feeds faked.
type testFixture struct {
	ctx     context.Context
	usecase *harnessUsecase
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
	engine   engineagents.Agents
	activity agentactivity.EventStore
	// providerPrefs is the real sqlite preference store the usecase resolves
	// providers against; setPrefs writes into it. connected is the injected install
	// probe's answer keyed by provider id; setConnected rewrites it. Both let the
	// provider-resolution tests control ordering/enabled/connected without touching
	// the host or a real PATH.
	providerPrefs store.Store[domain.AgentProviderPreference, string]
	connected     map[string]bool
	// minter is the SAME token minter the usecase's MCP seam verifies against, so
	// a test can mint the token a spawned runner would have been handed.
	minter *agenttools.TokenMinter
	// folders is the in-memory chat-folder table the lineage resolver reads, so a
	// test can file a thread inside folders and prove the walk steps through them.
	folders *mocks.AgentChatFolderStore
}

// fixtureChatReader adapts the chat EventStore into agenttools.ChatReader, whose
// Get is the store's GetChat under a shorter name.
type fixtureChatReader struct {
	chats agentchat.EventStore
}

func (r fixtureChatReader) Get(
	ctx context.Context,
	chatID string,
) (domain.Chat, error) {
	return r.chats.GetChat(ctx, chatID)
}

func (r fixtureChatReader) ListChats(
	ctx context.Context,
) ([]domain.Chat, error) {
	return r.chats.ListChats(ctx)
}

// fixtureWorkspaceLister answers for the single workspace the fixture spawns
// into ("ws1"): a plain child workspace, so the resolver's visibility set is
// exactly itself.
type fixtureWorkspaceLister struct{}

func (fixtureWorkspaceLister) Get(
	_ context.Context,
	wsID string,
) (domain.Workspace, error) {
	return domain.Workspace{ID: wsID, ProjectID: "p1", RepoID: "r1"}, nil
}

func (fixtureWorkspaceLister) List(
	_ context.Context,
) ([]domain.Workspace, error) {
	return []domain.Workspace{{ID: "ws1", ProjectID: "p1", RepoID: "r1"}}, nil
}

// fixtureReviewReader, fixtureThreadReader and fixtureThreadWriter exist only
// so DispatchMCP's tool surface registers post_review_comment,
// list_review_threads, get_review_scope, reply_to_review_thread and
// resolve_review_thread — none of these tests CALL a review tool, so every
// method here is an empty-returning stand-in. What matters is that the ports
// are non-nil: the full 8-tool surface has to come from the REAL concerns
// built through agentusecase.New for TestDispatchMCP_ListsTheChatTools to be a
// meaningful guard on New's own internal wiring (see that test's doc comment).
type fixtureReviewReader struct{}

func (fixtureReviewReader) GetScope(
	context.Context,
	domain.Workspace,
) (gitdomain.ReviewScope, error) {
	return gitdomain.ReviewScope{}, nil
}

func (fixtureReviewReader) GetOutline(
	context.Context,
	string,
	string,
) ([]gitdomain.FileOutline, error) {
	return nil, nil
}

type fixtureThreadReader struct{}

func (fixtureThreadReader) ListByWorkspace(context.Context, string) ([]domain.ReviewThread, error) {
	return nil, nil
}

func (fixtureThreadReader) Get(context.Context, string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

type fixtureThreadWriter struct{}

func (fixtureThreadWriter) Open(
	context.Context,
	reviewthread.OpenInput,
	time.Time,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (fixtureThreadWriter) Reply(
	context.Context,
	reviewthread.ReplyInput,
	time.Time,
) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

func (fixtureThreadWriter) Resolve(context.Context, string) (domain.ReviewThread, error) {
	return domain.ReviewThread{}, nil
}

// noopThreadBroadcast stands in for the app layer's hub fan-out. These tests
// never call a write tool, so nothing ever observes a broadcast; it only needs
// to be non-nil so canWriteReviewThread and canPostReviewComment register their
// tools.
func noopThreadBroadcast(domain.ReviewThread, string, string) {}

// setPrefs saves global provider preferences into the fixture's real store, so a
// following ResolveProviders reads them back.
func (f testFixture) setPrefs(
	t *testing.T,
	prefs ...domain.AgentProviderPreference,
) {
	t.Helper()
	for _, p := range prefs {
		require.NoError(t, f.providerPrefs.Save(f.ctx, p))
	}
}

// setConnected rewrites the injected probe's verdict, keyed by spawn.cmd (which
// equals the provider id for claude/codex/gemini). It mutates the map in place so
// the closure the usecase was built with sees the change.
func (f testFixture) setConnected(
	m map[string]bool,
) {
	for k := range f.connected {
		delete(f.connected, k)
	}
	for k, v := range m {
		f.connected[k] = v
	}
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
//
// transcript_path is part of "exactly": every real hook of a real conversation
// carries the rollout/transcript it belongs to (verified against codex 0.146.0 on a
// fresh start, on a resume, and on the internal memory session that DOESN'T have one
// — see the codex descriptor's require_payload_fields). mustJSON stamps it.
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
func (f testFixture) chat(t *testing.T, chatID string) domain.Chat {
	t.Helper()
	f.wait()
	c, err := f.chats.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	return c
}

// runner reads a live runner from the read model.
func (f testFixture) runner(t *testing.T, runnerID string) engineagents.Runner {
	t.Helper()
	f.wait()
	r, err := f.runners.Get(f.ctx, runnerID)
	require.NoError(t, err)
	return r
}

// liveRunnerFor answers "is this chat live, and who is on it" — the query that
// replaced ActiveSegmentID.
func (f testFixture) liveRunnerFor(t *testing.T, chatID string) (engineagents.Runner, error) {
	t.Helper()
	f.wait()
	return f.runners.LiveRunnerForChat(f.ctx, chatID)
}

// placedRunnersFor returns every live runner POINTED AT chatID. LiveRunnerForChat only
// answers "who holds it", so this is the invariant check: I2 says the answer must never
// be more than one, at any instant. A runner Crowbar has taken off a chat (Displace) is
// no longer pointed anywhere and so is not counted, even while its process is still
// dying.
func (f testFixture) placedRunnersFor(t *testing.T, chatID string) []engineagents.Runner {
	t.Helper()
	f.wait()
	all, err := f.runners.AllLive(f.ctx)
	require.NoError(t, err)
	var out []engineagents.Runner
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
	watch agentchat.WatchFunc,
) (agentchat.EventStore, func()) {
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

	repo, err := agentchat.NewEventSourced(ax, es, db, watch)
	require.NoError(t, err)
	return repo, ax.WaitPublish
}

// newActivityStore builds the REAL conversation record over an in-memory event
// log, read model and content directory. The fixture uses the real thing rather
// than a stub because the record is what every turn assertion in this package
// reads back, and a stub would let the write path and the read path agree with
// each other while both were wrong.
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

// newRunnerStore builds the same for the agentrunner aggregate: the real commands,
// the real live-runner + conversation-history projections, the real hub projection.
func newRunnerStore(
	t *testing.T,
	watch agentrunner.WatchFunc,
) (agentrunner.EventStore, func()) {
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

	repo, err := agentrunner.NewEventSourced(ax, es, db, watch)
	require.NoError(t, err)
	return repo, ax.WaitPublish
}

func newFixture(t *testing.T) testFixture {
	t.Helper()
	f, _, _ := newFixtureUsing(t, nil, nil, "")
	return f
}

// newFixtureWithPermissionDefault is newFixture with the fixture's pinned global
// permission default overridden — for the tests that need a chat spawned at
// something other than guarded.
func newFixtureWithPermissionDefault(t *testing.T, level string) testFixture {
	t.Helper()
	f, _, _ := newFixtureUsing(t, nil, nil, level)
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
		"",
	)
	return f, cs, rs
}

// newFixtureUsing builds a fixture; wrapChats/wrapRunners (if non-nil) adapt the real
// stores into the stores the usecase is built over. permissionDefault seeds the
// global permission-level preference the fixture starts with; the empty value
// means "use the package's own pinned default" ("guarded" — see below).
func newFixtureUsing(
	t *testing.T,
	wrapChats func(agentchat.EventStore) agentchat.EventStore,
	wrapRunners func(agentrunner.EventStore) agentrunner.EventStore,
	permissionDefault string,
	wrapActivity ...func(agentactivity.EventStore) agentactivity.EventStore,
) (testFixture, agentchat.EventStore, agentrunner.EventStore) {
	t.Helper()
	t.Setenv("CROWBAR_HOOK_BIN", "/fake/bin/crowbar")
	// codex.yaml is api-transport, and startAPIConn resolves the real `codex`
	// binary via binpath's well-known-dirs fallback regardless of PATH — a
	// developer machine with codex installed would otherwise fork a genuine
	// `codex app-server` subprocess as a side effect of spawning "codex" here.
	// See apiconn.go's own comment on this same variable.
	t.Setenv("CROWBAR_DISABLE_API_TRANSPORT", "1")

	bc := &fakeBroadcaster{}
	rbc := &fakeRunnerBroadcaster{}
	realChats, waitChats := newChatStore(t, bc.watchAgentChat)
	realRunners, waitRunners := newRunnerStore(t, rbc.watchAgentRunner)
	realActivity, waitActivity := newActivityStore(t)

	usedChats := realChats
	if wrapChats != nil {
		usedChats = wrapChats(realChats)
	}
	usedRunners := realRunners
	if wrapRunners != nil {
		usedRunners = wrapRunners(realRunners)
	}
	usedActivity := realActivity
	for _, wrap := range wrapActivity {
		usedActivity = wrap(usedActivity)
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

	engine := engineagents.New()
	providerPrefs, err := storesqlite.New[domain.AgentProviderPreference, string](":memory:")
	require.NoError(t, err)
	permissionPrefs, err := storesqlite.New[domain.AgentPermissionDefault, string](":memory:")
	require.NoError(t, err)
	// Pinned to guarded by default, not left at the shipped full-auto default:
	// SpawnChat seeds a spawned chat's level from whatever this global default
	// currently is (mirroring MintChat's own seed), and this package's tests
	// assert on that seeded value directly. permissionDefault overrides the
	// pin for the tests that need a chat seeded at something other than
	// guarded.
	pinnedDefault := permissionDefault
	if pinnedDefault == "" {
		pinnedDefault = "guarded"
	}
	require.NoError(t, permissionPrefs.Save(context.Background(), domain.AgentPermissionDefault{
		ID: domain.DefaultPermissionLevelKey, Level: pinnedDefault,
	}))
	connected := map[string]bool{}
	homeFn := func() (string, error) { return home, nil }
	probe := func(a engineagents.Agent) bool { return connected[a.ID()] }
	// The tool surface is wired over the SAME real stores the rest of the fixture
	// reads, so an MCP tool call lands in the aggregates every other test asserts
	// on. Only the workspace lister is a fake: these tests own no workspace
	// repository, and the resolver only needs the caller's workspace to exist.
	//
	// Every port that CAN be supplied here is, so the usecase this fixture builds
	// advertises the complete production tool surface — the same reason
	// agenttools' own toolsetOn fixture wires every port with stubs (see its doc
	// comment): a port left out here would silently narrow
	// TestDispatchMCP_ListsTheChatTools back to a vacuous guard.
	//
	// Chats and ChatLogs are NOT set in this Deps literal: both are self-assigned
	// by agentusecase.New once u exists (see its doc comment), the same chicken-and-egg
	// every caller of New faces, so wiring them here would just be re-doing what
	// New itself is responsible for — which is exactly the wiring
	// TestDispatchMCP_ListsTheChatTools exists to guard.
	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	chatReader := fixtureChatReader{chats: usedChats}
	resolver := agenttools.NewResolver(
		minter,
		usedRunners,
		chatReader,
		fixtureWorkspaceLister{},
	)
	// The REAL lineage resolver, over the same chat store and an in-memory folder
	// table, so a threaded chat in this package resolves its ancestors exactly the
	// way production does — folders and all. A stub here would have let the walk
	// and the spawn path agree with each other while both were wrong.
	folders := mocks.NewAgentChatFolderStore()
	lineage := tree.NewLineage(folders, usedChats)
	u := agentusecase.New(agentusecase.Deps{
		Chats:           usedChats,
		Runners:         usedRunners,
		Activity:        usedActivity,
		Agents:          engine,
		Terminal:        term,
		Workspace:       ws,
		Lineage:         lineage,
		ProviderPrefs:   providerPrefs,
		PermissionPrefs: permissionPrefs,
		Home:            homeFn,
		Installed:       probe,
		Minter:          minter,
		Tools: agenttools.Deps{
			Resolver:        resolver,
			ChatReads:       chatReader,
			Review:          fixtureReviewReader{},
			Threads:         fixtureThreadReader{},
			ThreadWrites:    fixtureThreadWriter{},
			Idempotency:     agenttools.NewIdempotency(),
			ThreadBroadcast: noopThreadBroadcast,
		},
	})
	// closeAssistantTurn's real 3s AwaitOpen wait only matters against a
	// concurrent delta, which resolves over its wake channel instantly, not by
	// waiting out the clock — so shrinking it here doesn't weaken any race
	// this package tests, it just stops every turn_stop-with-nothing-streamed
	// call from sitting idle for the full 3s.
	agentusecase.SetMessageAwaitTimeout(u, time.Millisecond)
	f := testFixture{
		ctx: context.Background(),
		usecase: &harnessUsecase{
			ChatUsecase:     u,
			TurnUsecase:     u,
			RunnerUsecase:   u,
			AnswerUsecase:   u,
			ProviderUsecase: u,
		},
		chats:         realChats,
		runners:       realRunners,
		waitFn:        func() { waitChats(); waitRunners(); waitActivity() },
		term:          term,
		bc:            bc,
		rbc:           rbc,
		ws:            ws,
		engine:        engine,
		activity:      realActivity,
		providerPrefs: providerPrefs,
		connected:     connected,
		minter:        minter,
		folders:       folders,
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
	_, err := f.runners.Move(f.ctx, runnerID, chatID, sessionID, false, time.Now())
	f.wait()
	return err
}

// faultActivity wraps the real conversation record so a test can arm a read
// failure. Only reads are faulted: the write paths in this package are already
// covered by the real store, and a fake that intercepted them would let the
// write path and the read path agree with each other while both were wrong.
type faultActivity struct {
	agentactivity.EventStore
	turnsErr   error
	choicesErr error
}

func (f *faultActivity) Choices(
	ctx context.Context, chatID string,
) ([]domain.ActivityChoice, error) {
	if f.choicesErr != nil {
		return nil, f.choicesErr
	}
	return f.EventStore.Choices(ctx, chatID)
}

func (f *faultActivity) PendingChoices(
	ctx context.Context, chatID string,
) ([]domain.ActivityChoice, error) {
	if f.choicesErr != nil {
		return nil, f.choicesErr
	}
	return f.EventStore.PendingChoices(ctx, chatID)
}

func (f *faultActivity) Turns(
	ctx context.Context, chatID string, after, before int64, limit int,
) ([]domain.ActivityTurn, error) {
	if f.turnsErr != nil {
		return nil, f.turnsErr
	}
	return f.EventStore.Turns(ctx, chatID, after, before, limit)
}

// newActivityFaultFixture builds a fixture whose usecase READS the conversation
// record through an armable wrapper.
func newActivityFaultFixture(t *testing.T) (testFixture, *faultActivity) {
	t.Helper()
	fa := &faultActivity{}
	f, _, _ := newFixtureUsing(t, nil, nil, "", func(real agentactivity.EventStore) agentactivity.EventStore {
		fa.EventStore = real
		return fa
	})
	return f, fa
}

// faultWriteActivity fails every WRITE, so the observation path can be asserted
// to degrade rather than to break the vendor CLI's turn.
type faultWriteActivity struct {
	agentactivity.EventStore
	writeErr error
}

func (f *faultWriteActivity) InvokeTool(ctx context.Context, in agentactivity.ToolInput) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.InvokeTool(ctx, in)
}

func (f *faultWriteActivity) CompleteTool(ctx context.Context, in agentactivity.ToolResultInput) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.CompleteTool(ctx, in)
}

func (f *faultWriteActivity) StartSubagent(ctx context.Context, chatID, id, agentType string, now time.Time) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.StartSubagent(ctx, chatID, id, agentType, now)
}

func (f *faultWriteActivity) StopSubagent(ctx context.Context, chatID, id, agentType string, now time.Time) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.StopSubagent(ctx, chatID, id, agentType, now)
}

func (f *faultWriteActivity) Interrupt(ctx context.Context, chatID, id, kind, detail string, now time.Time) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.Interrupt(ctx, chatID, id, kind, detail, now)
}

func (f *faultWriteActivity) ResolveInterruption(ctx context.Context, chatID, id, kind, detail string, now time.Time) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.ResolveInterruption(ctx, chatID, id, kind, detail, now)
}

func (f *faultWriteActivity) OpenChoice(ctx context.Context, in agentactivity.ChoiceInput) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.OpenChoice(ctx, in)
}

func (f *faultWriteActivity) ResolveChoice(ctx context.Context, chatID, choiceID, resolution string, now time.Time) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.ResolveChoice(ctx, chatID, choiceID, resolution, now)
}

func (f *faultWriteActivity) AnswerChoice(
	ctx context.Context, chatID, choiceID string, optionIDs []string, auto bool, now time.Time,
) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.AnswerChoice(ctx, chatID, choiceID, optionIDs, auto, now)
}

func newActivityWriteFaultFixture(t *testing.T) (testFixture, *faultWriteActivity) {
	t.Helper()
	fa := &faultWriteActivity{}
	f, _, _ := newFixtureUsing(t, nil, nil, "", func(real agentactivity.EventStore) agentactivity.EventStore {
		fa.EventStore = real
		return fa
	})
	return f, fa
}
