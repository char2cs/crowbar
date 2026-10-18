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
	"gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/adapter/store"
	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	agentchat "github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	agentactivity "github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity"
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
)

// mustJSON encodes a hook payload stamped with the transcript_path every real hook
// carries; a nil one marks codex's internal memory session. A payload naming no
// session is sent on the runner's own (harnessUsecase.IngestHook).
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

// fakeCommander is a thread-safe TerminalCommander double. A terminate ends the
// session as the engine does, running its onExit before it returns; a CLI dying on
// its own is f.term.exit(id). deadSessions backs SessionLive.
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
	exited       map[string]bool // sessions whose onExit has run
	// duringForkCall runs inside CreateCommand, without the mutex, before the
	// session id exists: the exact window in which a hook races a spawn.
	duringForkCall func(commandCall)
	// duringTerminate runs inside TerminateGraceful, without the mutex, as the
	// outgoing CLI is asked to die.
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

func (f *fakeCommander) TerminateGraceful(
	_ context.Context,
	sessionID string,
) error {
	if f.duringTerminate != nil {
		f.duringTerminate(sessionID)
	}

	f.mu.Lock()
	// terminateReq records EVERY attempt (even failed ones), so a best-effort caller
	// can assert the terminate was attempted regardless of outcome; terminated stays
	// success-only for callers that only care about the ids actually torn down.
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

// dieLocked marks sessionID dead and returns its onExit, nil once it has run:
// a session dies once.
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
	f.exitWith(t, sessionID, engineterminal.Exit{})
}

// exitWith is exit with the code and last screen the CLI died on.
func (f *fakeCommander) exitWith(t *testing.T, sessionID string, exit engineterminal.Exit) {
	t.Helper()
	f.mu.Lock()
	_, ok := f.byID[sessionID]
	// Copied out UNDER the lock: calls is appended to by CreateCommand on another
	// goroutine, so reading it after unlocking is a data race.
	onExit := f.dieLocked(sessionID)
	f.mu.Unlock()
	require.True(t, ok, "no spawned terminal session %q", sessionID)
	if onExit != nil {
		onExit(exit)
	}
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
	mu     sync.Mutex
	calls  []broadcastCall
	fanout *agentusecase.Fanout
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
	// mu guards the two call records: a probe and a switch resolve cwds concurrently.
	mu        sync.Mutex
	home      string
	projectID string
	repoID    string
	worktree  string
	chatsDir  string
	err       error
	// lastWorkspaceID records the id WorktreeDir was last called with. The
	// fake otherwise answers every id identically, including "" — a test
	// that only checks the CALL succeeded proves nothing about which
	// workspace a caller actually resolved a bubble's cwd against; a test
	// that needs that must read this field.
	lastWorkspaceID string
	// worktreeDirIDs records EVERY id WorktreeDir was called with, in order. A
	// verb that resolves a cwd more than once (SwitchProvider: a preflight, then
	// the spawn) can pass "" first and the ancestor's id second, which
	// lastWorkspaceID alone cannot tell apart from resolving both correctly.
	worktreeDirIDs []string
	worktreeErr    error // fails only WorktreeDir, leaving AgentChatsDir callers unaffected
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

// fakeWorktreeCreator is a thread-safe agentusecase.WorktreeCreator double:
// CreateChildWorkspace records the fork parent id it was called with and hands
// back a fresh workspace id, so Promote's tests can assert on both without a
// real worktree usecase (git engine, repo store) in this package's fixture.
type fakeWorktreeCreator struct {
	mu       sync.Mutex
	forkedOn []string
	// forkedBranches is the branch name each CreateChildWorkspace call asked
	// for, in the same order as forkedOn — "" for the server-generated-name
	// case, mirroring imported below.
	forkedBranches []string
	nextID         int
	err            error
	// discarded records the workspaces a failed promotion took back out, in
	// order. Without it a rollback that never ran and one that ran perfectly
	// look identical from the chat's side.
	discarded  []string
	discardErr error
	// imported records the branch each CreateImportedWorkspace call asked for,
	// in order, and importErr fails them. They are kept apart from forkedOn
	// above because the two verbs are the whole distinction this port now
	// carries: a fork names a PARENT to branch from, an import names the BRANCH
	// that already exists, and a test that could not tell them apart could not
	// prove an import took the import path.
	imported   []string
	importedWS domain.Workspace
	importErr  error
}

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

// imports returns the branches an import was asked for, in call order.
func (f *fakeWorktreeCreator) imports() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string{}, f.imported...)
}

func (f *fakeWorktreeCreator) DiscardChildWorkspace(
	_ context.Context,
	workspaceID string,
) error {
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

// fakeChatStore wraps a real agentchat.EventStore: a non-nil fail* field fails that
// one call. It records the chat ids AbandonTurn was called with, since that call is
// best-effort and a fault there would be invisible.
type fakeChatStore struct {
	agentchat.EventStore
	failGetChat         error
	failCreate          error
	failListChats       error
	failListByWorkspace error
	// failSetSelection / failLoadChat arm the two writes-and-reads the model and
	// effort selection travels through, so the "a spawn whose selection cannot be
	// read must fail before it forks" paths are reachable from a test.
	failSetSelection error
	failLoadChat     error
	// failSetWorkspace arms the one write that fills a promoted chat's
	// workspace slot, so the orphaned-workspace rollback is reachable.
	// failClearWorkspace arms the SAME write in the other direction — the
	// rollback's own first step — so the ordering that keeps the rollback safe
	// (never delete a workspace a row still points at) is reachable too.
	failSetWorkspace   error
	failClearWorkspace error
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

	// staleAsyncWorkProjection is staleProjection's twin for TURN STATE: GetChat
	// answers with the async-work level the chat had before its last turn_stop
	// folded, while LoadChat keeps answering from the log.
	//
	// It models the same ordinary state for the same reason. StopTurn is
	// deliberately on the async Send path (see the EventStore doc), so between a
	// turn_stop returning and its projection folding, the read model genuinely
	// still reports the level from before it. A restate that compares the open-work
	// level it just computed against THAT reads 0 == 0, calls itself a no-op, and
	// leaves the aggregate lit with nothing left to restate it.
	staleAsyncWorkProjection bool

	// onStopTurn runs INSIDE StopTurn, before the aggregate is written. It is the
	// only seam that can observe what a client would see the instant the turn-state
	// change is published, which is what the assistant-message ordering turns on.
	onStopTurn func()
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
	if s.staleAsyncWorkProjection {
		chat.AsyncWork = 0
		chat.Working = false
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

// ListByWorkspace is what the lineage resolver reads a spawn's ancestors
// through — folder rows and chat rows share this one list now — so this is
// the seam a test arms to prove a lineage read failure fails the spawn.
func (s *fakeChatStore) ListByWorkspace(ctx context.Context, workspaceID string) ([]domain.Chat, error) {
	if s.failListByWorkspace != nil {
		return nil, s.failListByWorkspace
	}
	return s.EventStore.ListByWorkspace(ctx, workspaceID)
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
	// sessionOf is the session runnerID's CLI is on.
	sessionOf func(runnerID string) string
	// own is the same usecase at its concrete type, for the seams reached only
	// through tree.Agent; snaps is the snapshot owner it was built over.
	own   *agentusecase.Usecase
	snaps *agentusecase.ChatSnapshots
}

// IngestHook delivers a hook as the runner's CLI sends it: every hook names a
// session, and one whose payload names none names the CLI's own.
func (h *harnessUsecase) IngestHook(ctx context.Context, runnerID, provider, kind string, payload []byte) error {
	return h.TurnUsecase.IngestHook(ctx, runnerID, provider, kind, h.onItsSession(runnerID, payload))
}

// IngestHookDelivery is IngestHook for a relayed hook.
func (h *harnessUsecase) IngestHookDelivery(
	ctx context.Context, deliveryID, runnerID, provider, kind string, payload []byte,
) error {
	return h.TurnUsecase.IngestHookDelivery(ctx, deliveryID, runnerID, provider, kind, h.onItsSession(runnerID, payload))
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

// testFixture is the usecase harness: the real asynx-backed chat AND runner
// aggregates (in-memory), with the terminal engine, the workspace reader and both
// hub feeds faked.
type testFixture struct {
	// snapFrames records every chat snapshot frame the owner publishes.
	snapFrames *snapshotFrames
	ctx        context.Context
	usecase    *harnessUsecase
	// chats is the REAL concrete EventStore, used for test reads; the usecase may
	// be built over a fault-injecting wrapper of it (newFaultFixture) but writes
	// still land here.
	chats agentchat.EventStore
	// waitFn drains the asynx dispatch queues and runs every projection handler
	// (ax.WaitPublish), so a subsequent read observes all prior mutations with no
	// polling and no timeouts.
	waitFn func()
	term   *fakeCommander
	bc     *fakeBroadcaster
	ws     *fakeWorkspace
	wt     *fakeWorktreeCreator
	// homeErr faults the crowbar-home resolver (Home), independent of ws.err —
	// see its construction in newFixtureUsing.
	homeErr  *error
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
	// folders/nodes are the Folder+Node fakes the lineage resolver reads
	// through (2026-09-08 sidebar-placement-unification Task 8) — a folder
	// is never a Chat row any more, so file() (below) seeds through these
	// instead of usedChats.Create.
	folders *mocks.FolderStore
	nodes   *mocks.NodePlacements
	ledger  *ledgerstore.Store
	// ledgerDB is the database under ledger, for faults no store method offers.
	ledgerDB *gorm.DB
	// switchGrace is what every boot of the usecase is given as its SwitchGrace.
	switchGrace *time.Duration
	// boot builds a daemon's usecase over the fixture's durable stores.
	boot func(*agentusecase.ChatSnapshots) *agentusecase.Usecase
	// sessions is the session id most recently announce()d per runnerID; a map,
	// so it is shared across every copy of this value-typed testFixture.
	sessions map[string]string
}

// sessionOf is the session runnerID's CLI is on: the one it announced, else the
// chat's bound session it was launched on, else one it has not announced yet.
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
	if sessionID != "" {
		f.sessions[runnerID] = sessionID
	}
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

// runner reads a live runner from the supervisor's live table.
func (f testFixture) runner(t *testing.T, runnerID string) engineagents.Runner {
	t.Helper()
	f.wait()
	r, ok := f.usecase.own.RunnerByID(runnerID)
	require.True(t, ok, "runner %s is live", runnerID)
	return r
}

// isLive reports whether runnerID is still in the live table.
func (f testFixture) isLive(runnerID string) bool {
	f.wait()
	_, ok := f.usecase.own.RunnerByID(runnerID)
	return ok
}

// liveRunnerFor answers "is this chat live, and who is on it".
func (f testFixture) liveRunnerFor(t *testing.T, chatID string) (engineagents.Runner, error) {
	t.Helper()
	f.wait()
	return f.usecase.own.LiveRunnerForChat(f.ctx, chatID)
}

// lastExit is how chatID's last runner ended, as its ledger records it.
func (f testFixture) lastExit(t *testing.T, chatID string) (ledger.RunnerExit, bool) {
	t.Helper()
	f.wait()
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	return st.LastExit()
}

// chatForSession is the chat that holds provider's sessionID, "" when none does.
func (f testFixture) chatForSession(t *testing.T, provider, sessionID string) string {
	t.Helper()
	f.wait()
	chatID, _, err := f.ledger.SessionOwner(f.ctx, provider, sessionID)
	require.NoError(t, err)
	return chatID
}

// markers are chatID's ledger markers as "kind:detail", oldest first.
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

// conversation is chatID's conversation with provider, as its ledger holds it.
func (f testFixture) conversation(t *testing.T, chatID, provider string) ledger.Conversation {
	t.Helper()
	f.wait()
	st, err := f.ledger.State(f.ctx, chatID)
	require.NoError(t, err)
	c, _ := st.Conversation(provider)
	return c
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

// runnerKinds is the kinds of the runner lifecycle frames published so far.
func (f testFixture) runnerKinds(t *testing.T) []string {
	t.Helper()
	f.wait()
	var kinds []string
	for _, fr := range f.snapFrames.all() {
		if fr.RunnerID != "" {
			kinds = append(kinds, fr.Kind)
		}
	}
	return kinds
}

// newChatStore builds a throwaway in-memory asynx-backed agentchat EventStore wired
// to the given hub broadcast func, returning it with its WaitPublish. broadcast is
// the SAME seam the production hub projection uses, so a test capturing frames
// through it exercises the real lifecycle feed — the usecase never broadcasts itself.
func newChatStore(
	t testing.TB,
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
func newActivityStore(tb testing.TB) (agentactivity.EventStore, func()) {
	tb.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(tb, err)
	ax, err := asynx.New[domain.ChatActivity]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(tb, err)
	tb.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(tb, err)

	repo, err := agentactivity.NewEventSourced(ax, es, db, tb.TempDir(), nil)
	require.NoError(tb, err)
	return repo, ax.WaitPublish
}

func newFixture(tb testing.TB) testFixture {
	tb.Helper()
	return newFixtureUsing(tb, nil, "")
}

// newFixtureWithPermissionDefault is newFixture with the fixture's pinned global
// permission default overridden — for the tests that need a chat spawned at
// something other than guarded.
func newFixtureWithPermissionDefault(t *testing.T, level string) testFixture {
	t.Helper()
	return newFixtureUsing(t, nil, level)
}

// newFaultFixture builds a fixture whose usecase writes/reads through fault-injecting
// wrappers the caller can arm; the fixture's own stores stay the real ones for reads.
func newFaultFixture(t *testing.T) (testFixture, *fakeChatStore) {
	t.Helper()
	cs := &fakeChatStore{}
	f := newFixtureUsing(t,
		func(real agentchat.EventStore) agentchat.EventStore { cs.EventStore = real; return cs },
		"",
	)
	return f, cs
}

// newFixtureUsing builds a fixture; wrapChats (if non-nil) adapts the real chat
// store into the store the usecase is built over. permissionDefault seeds the
// global permission-level preference the fixture starts with; the empty value
// means "use the package's own pinned default" ("guarded" — see below).
func newFixtureUsing(
	t testing.TB,
	wrapChats func(agentchat.EventStore) agentchat.EventStore,
	permissionDefault string,
	wrapActivity ...func(agentactivity.EventStore) agentactivity.EventStore,
) testFixture {
	t.Helper()
	t.Setenv("CROWBAR_HOOK_BIN", "/fake/bin/crowbar")
	// codex.yaml is api-transport, and startAPIConn resolves the real `codex`
	// binary via binpath's well-known-dirs fallback regardless of PATH — a
	// developer machine with codex installed would otherwise fork a genuine
	// `codex app-server` subprocess as a side effect of spawning "codex" here.
	// See apiconn.go's own comment on this same variable.
	t.Setenv("CROWBAR_DISABLE_API_TRANSPORT", "1")
	// The vendors' own homes, isolated: a spawn never sees a developer's real ones.
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	t.Setenv("CODEX_HOME", t.TempDir())

	snaps := agentusecase.NewChatSnapshots()
	snapFrames := &snapshotFrames{}
	snaps.SetPublish(snapFrames.record)
	fan := agentusecase.NewFanout(snaps)
	bc := &fakeBroadcaster{fanout: fan}
	realChats, waitChats := newChatStore(t, bc.watchAgentChat)
	realActivity, waitActivity := newActivityStore(t)

	usedChats := realChats
	if wrapChats != nil {
		usedChats = wrapChats(realChats)
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
	// removal is guarded by worktreepath.RemoveUnderHome (a chats dir NOT under home is refused).
	home := t.TempDir()
	// The daemon publishes where hooks are served before it spawns anything.
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
	// homeErr faults the crowbar-home resolver itself, distinct from f.ws.err.
	homeErr := new(error)
	homeFn := func() (string, error) {
		if *homeErr != nil {
			return "", *homeErr
		}
		return home, nil
	}
	probe := func(a engineagents.Agent) bool { return connected[a.ID()] }
	// The tool surface is wired over the SAME real stores the rest of the fixture
	// reads; its Resolver, Chats and ChatLogs are left to agentusecase.New, which
	// builds them: that wiring is what TestDispatchMCP_ListsTheChatTools guards.
	wt := &fakeWorktreeCreator{}
	minter, err := agenttools.NewTokenMinter()
	require.NoError(t, err)
	chatReader := fixtureChatReader{chats: usedChats}
	// The REAL lineage resolver over the same decorator production uses, so a
	// threaded chat resolves its folder ancestors exactly as it does there.
	folders := mocks.NewFolderStore()
	nodes := mocks.NewNodePlacements()
	lineage := tree.NewLineage(agentusecase.NewHomeCorrectedTreeChats(usedChats, nodes, folders))
	ledgerDB, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	chatLedger, err := ledgerstore.Open(ledgerDB, time.Now)
	require.NoError(t, err)
	switchGrace := new(time.Duration)
	boot := func(snaps *agentusecase.ChatSnapshots) *agentusecase.Usecase {
		return bootUsecase(t, agentusecase.Deps{
			SwitchGrace:     *switchGrace,
			Chats:           usedChats,
			Activity:        usedActivity,
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
			Tools: agenttools.Deps{
				Scope:           fixtureWorkspaceLister{},
				ChatReads:       chatReader,
				Review:          fixtureReviewReader{},
				Threads:         fixtureThreadReader{},
				ThreadWrites:    fixtureThreadWriter{},
				Idempotency:     agenttools.NewIdempotency(),
				ThreadBroadcast: noopThreadBroadcast,
			},
			Snapshots: snaps,
		})
	}
	u := boot(snaps)
	sessions := map[string]string{}
	f := testFixture{
		ctx:           context.Background(),
		usecase:       newHarnessUsecase(u, snaps, chatLedger, sessions),
		boot:          boot,
		chats:         realChats,
		waitFn:        func() { waitChats(); waitActivity() },
		term:          term,
		bc:            bc,
		ws:            ws,
		wt:            wt,
		homeErr:       homeErr,
		engine:        engine,
		activity:      realActivity,
		providerPrefs: providerPrefs,
		connected:     connected,
		minter:        minter,
		folders:       folders,
		nodes:         nodes,
		sessions:      sessions,
		snapFrames:    snapFrames,
		ledger:        chatLedger,
		ledgerDB:      ledgerDB,
		switchGrace:   switchGrace,
	}
	return f
}

// withSwitchGrace is f booted with d as how long a provider switch waits on the
// outgoing turn before forcing it, as every later restart is too.
func (f testFixture) withSwitchGrace(t *testing.T, d time.Duration) testFixture {
	t.Helper()
	*f.switchGrace = d
	f.restart(t)
	return f
}

func bootUsecase(tb testing.TB, d agentusecase.Deps) *agentusecase.Usecase {
	tb.Helper()
	u := agentusecase.New(d)
	// closeAssistantTurn's AwaitOpen wait only matters against a concurrent
	// delta, which resolves over its wake channel, never by the clock.
	agentusecase.SetMessageAwaitTimeout(u, time.Millisecond)
	// The supervisor's background work ends before the stores it writes close.
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

// restart is the daemon coming back over the same durable stores: every PTY
// died with the old one, no exit callback ran, and nothing in memory survives.
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

// refuseRunnerStarted makes the ledger refuse every runner_started row, as a
// failing disk refuses a write: the CLI is forked, its record is not.
func (f testFixture) refuseRunnerStarted(t *testing.T) {
	t.Helper()
	require.NoError(t, f.ledgerDB.Exec(`CREATE TRIGGER refuse_runner_started BEFORE INSERT ON chat_ledger
		WHEN NEW.kind = 'runner_started' BEGIN SELECT RAISE(ABORT, 'boom: disk refused'); END`).Error)
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
	f := newFixtureUsing(t, nil, "", func(real agentactivity.EventStore) agentactivity.EventStore {
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

func (f *faultWriteActivity) StopSubagent(ctx context.Context, chatID, id, agentType, message string, now time.Time) error {
	if f.writeErr != nil {
		return f.writeErr
	}
	return f.EventStore.StopSubagent(ctx, chatID, id, agentType, message, now)
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
	f := newFixtureUsing(t, nil, "", func(real agentactivity.EventStore) agentactivity.EventStore {
		fa.EventStore = real
		return fa
	})
	return f, fa
}

// snapshotFrames records every frame the chat snapshot owner publishes.
type snapshotFrames struct {
	mu     sync.Mutex
	frames []agentusecase.ChatSnapshotFrame
}

func (r *snapshotFrames) record(f agentusecase.ChatSnapshotFrame) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = append(r.frames, f)
}

// all returns every frame published, oldest first.
func (r *snapshotFrames) all() []agentusecase.ChatSnapshotFrame {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]agentusecase.ChatSnapshotFrame{}, r.frames...)
}

// reset drops the frames recorded so far.
func (r *snapshotFrames) reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.frames = nil
}

// forChat returns the frames published about chatID, oldest first.
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
