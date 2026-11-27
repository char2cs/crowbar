package chat_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/tree"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

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
	chatID string
	cwd    string
	argv   []string
	env    []string
	onExit func(engineterminal.Exit)
}

type fakeCommander struct {
	mu              sync.Mutex
	calls           []commandCall
	terminated      []string
	terminateReq    []string
	byID            map[string]int
	nextID          int
	err             error
	terminateErr    error
	deadSessions    map[string]bool
	exited          map[string]bool
	duringForkCall  func(commandCall)
	duringTerminate func(sessionID string)
}

func (f *fakeCommander) CreateCommand(
	_ context.Context,
	chatID string,
	cwd string,
	argv []string,
	env []string,
	onExit func(engineterminal.Exit),
) (string, error) {
	if f.duringForkCall != nil {
		f.duringForkCall(commandCall{
			chatID: chatID,
			cwd:    cwd,
			argv:   append([]string{}, argv...),
			env:    append([]string{}, env...),
			onExit: onExit,
		})
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
		chatID: chatID,
		cwd:    cwd,
		argv:   append([]string{}, argv...),
		env:    append([]string{}, env...),
		onExit: onExit,
	})
	return id, nil
}

func (f *fakeCommander) Write(_ context.Context, _ string, _ []byte) error { return nil }

func (f *fakeCommander) TerminateGraceful(
	_ context.Context,
	sessionID string,
) error {
	if f.duringTerminate != nil {
		f.duringTerminate(sessionID)
	}

	f.mu.Lock()
	f.terminateReq = append(f.terminateReq, sessionID)
	if f.terminateErr != nil {
		f.mu.Unlock()
		return f.terminateErr
	}
	f.terminated = append(f.terminated, sessionID)
	onExit := f.dieLocked(sessionID)
	f.mu.Unlock()
	if onExit != nil {
		onExit(engineterminal.Exit{})
	}
	return nil
}

func (f *fakeCommander) dieLocked(sessionID string) func(engineterminal.Exit) {
	if f.deadSessions == nil {
		f.deadSessions = map[string]bool{}
	}
	if f.exited == nil {
		f.exited = map[string]bool{}
	}
	f.deadSessions[sessionID] = true
	idx, ok := f.byID[sessionID]
	if !ok || f.exited[sessionID] {
		return nil
	}
	f.exited[sessionID] = true
	return f.calls[idx].onExit
}

func (f *fakeCommander) SessionLive(_ context.Context, sessionID string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return !f.deadSessions[sessionID]
}

func (f *fakeCommander) exit(t *testing.T, sessionID string) {
	t.Helper()
	f.exitWith(t, sessionID, engineterminal.Exit{})
}

func (f *fakeCommander) exitWith(t *testing.T, sessionID string, exit engineterminal.Exit) {
	t.Helper()
	f.mu.Lock()
	_, ok := f.byID[sessionID]
	onExit := f.dieLocked(sessionID)
	f.mu.Unlock()
	require.True(t, ok, "no spawned terminal session %q", sessionID)
	if onExit != nil {
		onExit(exit)
	}
}

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

func (f *fakeCommander) terminatedIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.terminated...)
}

func (f *fakeCommander) terminateRequestIDs() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.terminateReq...)
}

type broadcastCall struct {
	chatID      string
	workspaceID string
	kind        string
}

type fakeBroadcaster struct {
	mu     sync.Mutex
	calls  []broadcastCall
	fanout *agentusecase.Fanout
}

func (f *fakeBroadcaster) BroadcastAgentChatFolder(_, _, _ string) {}

func (f *fakeBroadcaster) BroadcastAgentChat(chatID, workspaceID, kind string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, broadcastCall{chatID: chatID, workspaceID: workspaceID, kind: kind})
}

func (f *fakeBroadcaster) reset() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = nil
}

func (f *fakeBroadcaster) watchAgentChat(e agentchat.ChatEvent) {
	f.mu.Lock()
	fan := f.fanout
	f.mu.Unlock()
	if fan != nil {
		fan.ChatWatch()(e)
	}
}

func (f *fakeBroadcaster) setFanout(fan *agentusecase.Fanout) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fanout = fan
}

func (f *fakeBroadcaster) snapshot() []broadcastCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]broadcastCall{}, f.calls...)
}

type fakeWorkspace struct {
	mu              sync.Mutex
	home            string
	projectID       string
	repoID          string
	worktree        string
	chatsDir        string
	err             error
	lastWorkspaceID string
	worktreeDirIDs  []string
	worktreeErr     error
}

func (f *fakeWorkspace) WorktreeDir(
	_ context.Context,
	workspaceID string,
) (crowbarHome, projectID, repoID, worktree string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastWorkspaceID = workspaceID
	f.worktreeDirIDs = append(f.worktreeDirIDs, workspaceID)
	if f.err != nil {
		return "", "", "", "", f.err
	}
	if f.worktreeErr != nil {
		return "", "", "", "", f.worktreeErr
	}
	return f.home, f.projectID, f.repoID, f.worktree, nil
}

func (f *fakeWorkspace) AgentChatsDir(
	_ context.Context,
	workspaceID string,
) (string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.lastWorkspaceID = workspaceID
	if f.err != nil {
		return "", f.err
	}
	return f.chatsDir, nil
}

type fakeWorktreeCreator struct {
	mu             sync.Mutex
	forkedOn       []string
	forkedBranches []string
	nextID         int
	err            error
	discarded      []string
	discardErr     error
	imported       []string
	importedWS     domain.Workspace
	importErr      error
}

func (f *fakeWorktreeCreator) CreateBranchOutWorkspace(ctx context.Context, parentID, branch string) (domain.Workspace, error) {
	return f.CreateChildWorkspace(ctx, parentID, branch)
}

func (f *fakeWorktreeCreator) CheckBranchOutName(context.Context, string, string) error { return nil }

func (f *fakeWorktreeCreator) CreateChildWorkspace(
	_ context.Context,
	forkParentID string,
	branch string,
) (domain.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.forkedOn = append(f.forkedOn, forkParentID)
	f.forkedBranches = append(f.forkedBranches, branch)
	if f.err != nil {
		return domain.Workspace{}, f.err
	}
	f.nextID++
	return domain.Workspace{ID: fmt.Sprintf("ws-child-%d", f.nextID)}, nil
}

func (f *fakeWorktreeCreator) CreateImportedWorkspace(
	_ context.Context,
	spec agentusecase.ImportSpec,
) (domain.Workspace, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.imported = append(f.imported, spec.Branch)
	if f.importErr != nil {
		return domain.Workspace{}, f.importErr
	}
	if f.importedWS.ID != "" {
		return f.importedWS, nil
	}
	f.nextID++
	return domain.Workspace{ID: fmt.Sprintf("ws-child-%d", f.nextID)}, nil
}

func (f *fakeWorktreeCreator) imports() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.imported...)
}

func (f *fakeWorktreeCreator) DiscardChildWorkspace(_ context.Context, workspaceID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.discarded = append(f.discarded, workspaceID)
	return f.discardErr
}

func (f *fakeWorktreeCreator) discards() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.discarded...)
}

func (f *fakeWorktreeCreator) calls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.forkedOn...)
}

type fakeChatStore struct {
	agentchat.EventStore
	failGetChat         error
	failCreate          error
	failListChats       error
	failListByWorkspace error
	failSetSelection    error
	failLoadChat        error
	failSetWorkspace    error
	failClearWorkspace  error
	failLoadChatAfter   int
	staleProjection     bool
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
	if err != nil {
		return domain.Chat{}, err
	}
	if s.staleProjection {
		chat.ParentID = ""
	}
	return chat, nil
}

func (s *fakeChatStore) SetWorkspace(ctx context.Context, id, workspaceID string) (domain.Chat, error) {
	if s.failSetWorkspace != nil && workspaceID != "" {
		return domain.Chat{}, s.failSetWorkspace
	}
	if s.failClearWorkspace != nil && workspaceID == "" {
		return domain.Chat{}, s.failClearWorkspace
	}
	return s.EventStore.SetWorkspace(ctx, id, workspaceID)
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

func (s *fakeChatStore) ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.Chat, error) {
	if s.failListByWorkspace != nil {
		return nil, s.failListByWorkspace
	}
	return s.EventStore.ListByWorkspace(ctx, workspaceID)
}

type harnessUsecase struct {
	agentusecase.ChatUsecase
	agentusecase.TurnUsecase
	agentusecase.RunnerUsecase
	agentusecase.AnswerUsecase
	agentusecase.ProviderUsecase
	sessionOf func(runnerID string) string
	own       *agentusecase.Usecase
	snaps     *agentusecase.ChatSnapshots
}

func (h *harnessUsecase) IngestHook(ctx context.Context, runnerID, provider, kind string, payload []byte) error {
	_, err := h.placeHook(ctx, engineagents.ChannelHooks, runnerID, provider, kind, payload)
	return err
}

func (h *harnessUsecase) placeHook(
	ctx context.Context, channel engineagents.Channel, runnerID, provider, kind string, payload []byte,
) (agentusecase.HookReply, error) {
	return h.Hook(ctx, runnerID, provider, kind, h.onItsSession(runnerID, payload), channel)
}

func (h *harnessUsecase) onItsSession(runnerID string, payload []byte) []byte {
	var m map[string]any
	if json.Unmarshal(payload, &m) != nil {
		return payload
	}
	if _, set := m["session_id"]; set {
		return payload
	}
	m["session_id"] = h.sessionOf(runnerID)
	out, err := json.Marshal(m)
	if err != nil {
		return payload
	}
	return out
}

type testFixture struct {
	snapFrames    *snapshotFrames
	ctx           context.Context
	usecase       *harnessUsecase
	chats         agentchat.EventStore
	waitFn        func()
	term          *fakeCommander
	bc            *fakeBroadcaster
	ws            *fakeWorkspace
	wt            *fakeWorktreeCreator
	homeErr       *error
	engine        engineagents.Agents
	activity      activityReads
	providerPrefs store.Store[domain.AgentProviderPreference, string]
	connected     map[string]bool
	minter        *agenttools.TokenMinter
	folders       *mocks.FolderStore
	nodes         *mocks.NodePlacements
	ledger        *ledgerstore.Store
	ledgerDB      *gorm.DB
	boot          func(*agentusecase.ChatSnapshots) *agentusecase.Usecase
	sessions      map[string]string
}

func sessionOf(u *agentusecase.Usecase, l *ledgerstore.Store, announced map[string]string) func(string) string {
	return func(runnerID string) string {
		if id, ok := announced[runnerID]; ok {
			return id
		}
		r, ok := u.RunnerByID(runnerID)
		if !ok {
			return "unannounced"
		}
		st, err := l.State(context.Background(), r.CurrentChatID)
		if c, ok := st.Conversation(r.ProviderID); err == nil && ok && c.State == ledger.ConversationBound {
			return c.SessionID
		}
		return "unannounced"
	}
}

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

func noopThreadBroadcast(domain.ReviewThread, string, string) {}

func (f testFixture) setPrefs(
	t *testing.T,
	prefs ...domain.AgentProviderPreference,
) {
	t.Helper()
	for _, p := range prefs {
		require.NoError(t, f.providerPrefs.Save(f.ctx, p))
	}
}

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

func (f testFixture) wait() {
	f.waitFn()
}

func (f testFixture) spawnChat(ctx context.Context, provider string) (string, string, error) {
	chatID, err := f.usecase.MintChat(ctx, "", "ws1", provider, "")
	if err != nil {
		return "", "", err
	}
	runnerID, err := f.usecase.StartRunner(ctx, chatID, provider)
	return chatID, runnerID, err
}

func (f testFixture) spawn(t *testing.T, provider string) (chatID, runnerID string) {
	t.Helper()
	chatID, runnerID, err := f.spawnChat(f.ctx, provider)
	require.NoError(t, err)
	f.wait()
	return chatID, runnerID
}

func (f testFixture) announce(t *testing.T, runnerID, sessionID string) {
	t.Helper()
	if sessionID != "" {
		f.sessions[runnerID] = sessionID
	}
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, "", "session_start",
		mustJSON(t, map[string]any{"session_id": sessionID})))
	f.wait()
}

func turn(t *testing.T, f testFixture, runnerID, provider, content string) {
	t.Helper()
	require.NoError(t, f.usecase.IngestHook(f.ctx, runnerID, provider, "turn_stop",
		mustJSON(t, map[string]any{"last_assistant_message": content})))
	f.wait()
}

func (f testFixture) chat(t *testing.T, chatID string) domain.Chat {
	t.Helper()
	f.wait()
	c, err := f.chats.GetChat(f.ctx, chatID)
	require.NoError(t, err)
	return c
}

func (f testFixture) busy(t *testing.T, chatID string) bool {
	t.Helper()
	f.wait()
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	return st.Busy()
}

func (f testFixture) runner(t *testing.T, runnerID string) engineagents.Runner {
	t.Helper()
	f.wait()
	r, ok := f.usecase.own.RunnerByID(runnerID)
	require.True(t, ok, "runner %s is live", runnerID)
	return r
}

func (f testFixture) isLive(runnerID string) bool {
	f.wait()
	_, ok := f.usecase.own.RunnerByID(runnerID)
	return ok
}

func (f testFixture) liveRunnerFor(t *testing.T, chatID string) (engineagents.Runner, error) {
	t.Helper()
	f.wait()
	return f.usecase.own.LiveRunnerForChat(f.ctx, chatID)
}

func (f testFixture) lastExit(t *testing.T, chatID string) (ledger.RunnerExit, bool) {
	t.Helper()
	f.wait()
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	return st.LastExit()
}

func (f testFixture) chatForSession(t *testing.T, provider, sessionID string) string {
	t.Helper()
	f.wait()
	chatID, _, err := f.ledger.SessionOwner(f.ctx, provider, sessionID)
	require.NoError(t, err)
	return chatID
}

func (f testFixture) markers(t *testing.T, chatID string) []string {
	t.Helper()
	f.wait()
	rows, err := f.ledger.Read(f.ctx, chatID, 0, 1000)
	require.NoError(t, err)
	var out []string
	for _, r := range rows {
		if r.Kind != ledger.KindMarker {
			continue
		}
		m, err := ledger.DecodeBody[ledger.Marker](r)
		require.NoError(t, err)
		out = append(out, m.Kind+":"+m.Detail)
	}
	return out
}

func (f testFixture) conversation(t *testing.T, chatID, provider string) ledger.Conversation {
	t.Helper()
	f.wait()
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	c, _ := st.Conversation(provider)
	return c
}

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

func (f testFixture) runnerKinds(t *testing.T) []string {
	t.Helper()
	f.wait()
	f.snapFrames.mu.Lock()
	defer f.snapFrames.mu.Unlock()
	return append([]string(nil), f.snapFrames.runner...)
}

func newChatStore(
	t testing.TB,
	watch agentchat.WatchFunc,
) (agentchat.EventStore, func()) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	rm, err := agentchat.OpenReadModel(db)
	require.NoError(t, err)
	axb := asynx.New[domain.Chat]().
		WithEventStore(es).WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	agentchat.Subscriber(rm)(axb)
	ax, err := axb.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	repo, err := agentchat.NewEventSourced(context.Background(), ax, es, rm, watch, testutil.Placements(t))
	require.NoError(t, err)
	return repo, ax.WaitPublish
}

func newFixture(tb testing.TB) testFixture {
	tb.Helper()
	return newFixtureUsing(tb, nil, "")
}

func newFixtureWithPermissionDefault(t *testing.T, level string) testFixture {
	t.Helper()
	return newFixtureUsing(t, nil, level)
}

func newFaultFixture(t *testing.T) (testFixture, *fakeChatStore) {
	t.Helper()
	cs := &fakeChatStore{}
	f := newFixtureUsing(t,
		func(real agentchat.EventStore) agentchat.EventStore { cs.EventStore = real; return cs },
		"",
	)
	return f, cs
}

func newFixtureUsing(
	t testing.TB,
	wrapChats func(agentchat.EventStore) agentchat.EventStore,
	permissionDefault string,
) testFixture {
	t.Helper()
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())

	snaps := agentusecase.NewChatSnapshots()
	snapFrames := &snapshotFrames{}
	snaps.SetPublish(snapFrames.record)
	fan := agentusecase.NewFanout(snaps)
	bc := &fakeBroadcaster{fanout: fan}
	snapFrames.chats = bc
	realChats, waitChats := newChatStore(t, bc.watchAgentChat)

	usedChats := realChats
	if wrapChats != nil {
		usedChats = wrapChats(realChats)
	}

	term := &fakeCommander{}
	home := t.TempDir()
	require.NoError(t, hookserver.Publish(home, "http://127.0.0.1:4242"))
	worktree := filepath.Join(home, "projects", "p1", "slug", "branch", "worktree")
	ws := &fakeWorkspace{
		home:      home,
		projectID: "p1",
		repoID:    "r1",
		worktree:  worktree,
		chatsDir:  worktreepath.ChatsDir(worktree),
	}

	engine := engineagents.New()
	t.Cleanup(engine.Close)
	providerPrefs, err := storesqlite.New[domain.AgentProviderPreference, string](":memory:")
	require.NoError(t, err)
	permissionPrefs, err := storesqlite.New[domain.AgentPermissionDefault, string](":memory:")
	require.NoError(t, err)
	pinnedDefault := permissionDefault
	if pinnedDefault == "" {
		pinnedDefault = "guarded"
	}
	require.NoError(t, permissionPrefs.Save(context.Background(), domain.AgentPermissionDefault{
		ID: domain.DefaultPermissionLevelKey, Level: pinnedDefault,
	}))
	connected := map[string]bool{}
	homeErr := new(error)
	homeFn := func() (string, error) {
		if *homeErr != nil {
			return "", *homeErr
		}
		return home, nil
	}
	probe := func(a engineagents.Agent) bool { return connected[a.ID()] }
	wt := &fakeWorktreeCreator{}
	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	chatReader := fixtureChatReader{chats: usedChats}
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	lineage := tree.NewLineage(agentusecase.NewHomeCorrectedTreeChats(usedChats, nodes, folders))
	ledgerDB, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	chatLedger, err := ledgerstore.Open(t.Context(), ledgerDB, time.Now)
	require.NoError(t, err)
	blobs, err := ledgerstore.OpenBlobs(t.TempDir())
	require.NoError(t, err)
	boot := func(snaps *agentusecase.ChatSnapshots) *agentusecase.Usecase {
		return bootUsecase(t, agentusecase.Deps{
			Chats:           usedChats,
			Blobs:           blobs,
			Agents:          engine,
			Terminal:        term,
			Workspace:       ws,
			Worktree:        wt,
			Lineage:         lineage,
			ProviderPrefs:   providerPrefs,
			PermissionPrefs: permissionPrefs,
			Home:            homeFn,
			Ledger:          chatLedger,
			Installed:       probe,
			Minter:          minter,
			NoAPITransport:  true,
			Tools: agenttools.Deps{
				Scope:           fixtureWorkspaceLister{},
				ChatReads:       chatReader,
				Review:          fixtureReviewReader{},
				Threads:         fixtureThreadReader{},
				ThreadWrites:    fixtureThreadWriter{},
				ThreadChanges:   reviewthread.NewChanges(),
				Idempotency:     agenttools.NewIdempotency(),
				ThreadBroadcast: noopThreadBroadcast,
			},
			Snapshots:   snaps,
			AnswerClock: time.After,
		})
	}
	u := boot(snaps)
	sessions := map[string]string{}
	harness := newHarnessUsecase(u, snaps, chatLedger, sessions)
	f := testFixture{
		ctx:           context.Background(),
		usecase:       harness,
		boot:          boot,
		chats:         realChats,
		waitFn:        waitChats,
		term:          term,
		bc:            bc,
		ws:            ws,
		wt:            wt,
		homeErr:       homeErr,
		engine:        engine,
		activity:      activityReads{u: harness, ledger: chatLedger, blobs: blobs},
		providerPrefs: providerPrefs,
		connected:     connected,
		minter:        minter,
		folders:       folders,
		nodes:         nodes,
		sessions:      sessions,
		snapFrames:    snapFrames,
		ledger:        chatLedger,
		ledgerDB:      ledgerDB,
	}
	return f
}

func bootUsecase(tb testing.TB, d agentusecase.Deps) *agentusecase.Usecase {
	tb.Helper()
	u := agentusecase.New(d)
	tb.Cleanup(u.ShutdownAPIConnections)
	return u
}

func newHarnessUsecase(
	u *agentusecase.Usecase, snaps *agentusecase.ChatSnapshots, l *ledgerstore.Store, sessions map[string]string,
) *harnessUsecase {
	return &harnessUsecase{
		own:             u,
		snaps:           snaps,
		ChatUsecase:     u,
		TurnUsecase:     u,
		RunnerUsecase:   u,
		AnswerUsecase:   u,
		ProviderUsecase: u,
		sessionOf:       sessionOf(u, l, sessions),
	}
}

func (f testFixture) restart(t *testing.T) {
	t.Helper()
	f.term.dieWithDaemon()
	f.wait()
	f.usecase.snaps.SetPublish(func(agentusecase.ChatSnapshotFrame) {})
	f.snapFrames.reset()
	snaps := agentusecase.NewChatSnapshots()
	snaps.SetPublish(f.snapFrames.record)
	f.bc.setFanout(agentusecase.NewFanout(snaps))
	*f.usecase = *newHarnessUsecase(f.boot(snaps), snaps, f.ledger, f.sessions)
}

func (f testFixture) refuseRunnerStarted(t *testing.T) {
	t.Helper()
	require.NoError(t, f.ledgerDB.Exec(`CREATE TRIGGER refuse_runner_started BEFORE INSERT ON chat_ledger
		WHEN NEW.kind = 'runner_started' BEGIN SELECT RAISE(ABORT, 'boom: disk refused'); END`).Error)
}

func (f testFixture) refuseRows(t *testing.T, kinds ...ledger.Kind) {
	t.Helper()
	quoted := make([]string, 0, len(kinds))
	for _, k := range kinds {
		quoted = append(quoted, "'"+string(k)+"'")
	}
	require.NoError(t, f.ledgerDB.Exec(`CREATE TRIGGER refuse_rows BEFORE INSERT ON chat_ledger
		WHEN NEW.kind IN (`+strings.Join(quoted, ",")+`) BEGIN SELECT RAISE(ABORT, 'log is full'); END`).Error)
}

type snapshotFrames struct {
	mu     sync.Mutex
	frames []agentusecase.ChatSnapshotFrame
	runner []string
	last   map[string]agentusecase.ChatSnapshotFrame
	chats  *fakeBroadcaster
}

func (r *snapshotFrames) record(f agentusecase.ChatSnapshotFrame) {
	r.mu.Lock()
	r.frames = append(r.frames, f)
	if r.last == nil {
		r.last = map[string]agentusecase.ChatSnapshotFrame{}
	}
	id := f.Snapshot.Chat.ID
	prev, seen := r.last[id]
	r.last[id] = f
	chatChanges, runnerChanges := changesOf(prev, seen, f)
	r.runner = append(r.runner, runnerChanges...)
	chats := r.chats
	r.mu.Unlock()
	for _, kind := range chatChanges {
		if chats != nil {
			chats.BroadcastAgentChat(id, f.Snapshot.Chat.WorkspaceID, kind)
		}
	}
}

func changesOf(prev agentusecase.ChatSnapshotFrame, seen bool, f agentusecase.ChatSnapshotFrame) (chat, runner []string) {
	was, now := prev.Snapshot, f.Snapshot
	if f.Deleted {
		return []string{"deleted"}, nil
	}
	if !seen {
		chat = append(chat, "created")
	}
	if seen && was.Chat.PermissionLevel != now.Chat.PermissionLevel || !seen && now.Chat.PermissionLevel != "" {
		chat = append(chat, "permission_level_set")
	}
	if seen && was.Chat.ProviderID != now.Chat.ProviderID {
		chat = append(chat, "provider_set")
	}
	if was.Chat.Title != now.Chat.Title {
		chat = append(chat, "title_set")
	}
	if was.Ledger.Busy != now.Ledger.Busy {
		chat = append(chat, map[bool]string{true: "turn_started", false: "turn_stopped"}[now.Ledger.Busy])
	}
	if now.Ledger.LastExit != nil && (was.Ledger.LastExit == nil || *was.Ledger.LastExit != *now.Ledger.LastExit) {
		runner = append(runner, "exited")
	}
	if now.Live != nil && (was.Live == nil || was.Live.ID != now.Live.ID) {
		runner = append(runner, "started")
	}
	return chat, runner
}

func (r *snapshotFrames) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames, r.runner = nil, nil
}

func (r *snapshotFrames) forChat(chatID string) []agentusecase.ChatSnapshotFrame {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []agentusecase.ChatSnapshotFrame
	for _, f := range r.frames {
		if f.Snapshot.Chat.ID == chatID {
			out = append(out, f)
		}
	}
	return out
}
