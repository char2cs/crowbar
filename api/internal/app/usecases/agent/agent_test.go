package agent_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

type fakeCommander struct {
	calls        []commandCall
	terminated   []string
	nextID       int
	err          error
	terminateErr error
}

func (f *fakeCommander) CreateCommand(
	_ context.Context,
	workspaceID string,
	cwd string,
	argv []string,
	env []string,
	onExit func(),
) (string, error) {
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
	if f.terminateErr != nil {
		return f.terminateErr
	}
	f.terminated = append(f.terminated, sessionID)
	return nil
}

type broadcastCall struct {
	chatID string
	kind   string
}

type fakeBroadcaster struct {
	calls []broadcastCall
}

func (f *fakeBroadcaster) BroadcastAgentChat(chatID, kind string) {
	f.calls = append(f.calls, broadcastCall{chatID: chatID, kind: kind})
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

// erroringStore wraps a real agentchat.Store, letting a test force the Nth
// call to a given method to fail so the usecase's error-wrap guard clauses
// are exercised without a fault-injecting database. A zero "at" value never
// fails; a positive one fails only that 1-indexed call, so a test can target
// e.g. spawnSegment's second SaveSegment call (persisting TerminalSessionID)
// without also breaking its first (persisting the fresh segment row).
type erroringStore struct {
	agentchat.Store
	saveChatCalls    int
	saveSegmentCalls int
	getChatCalls     int
	failSaveChatAt   int
	failSaveSegAt    int
	failGetChatAt    int
	failAllSegments  bool
}

func (s *erroringStore) SaveChat(ctx context.Context, c domain.AgentChat) error {
	s.saveChatCalls++
	if s.failSaveChatAt != 0 && s.saveChatCalls == s.failSaveChatAt {
		return fmt.Errorf("boom: save chat")
	}
	return s.Store.SaveChat(ctx, c)
}

func (s *erroringStore) SaveSegment(ctx context.Context, seg domain.AgentSegment) error {
	s.saveSegmentCalls++
	if s.failSaveSegAt != 0 && s.saveSegmentCalls == s.failSaveSegAt {
		return fmt.Errorf("boom: save segment")
	}
	return s.Store.SaveSegment(ctx, seg)
}

func (s *erroringStore) GetChat(ctx context.Context, id string) (domain.AgentChat, error) {
	s.getChatCalls++
	if s.failGetChatAt != 0 && s.getChatCalls == s.failGetChatAt {
		return domain.AgentChat{}, fmt.Errorf("boom: get chat")
	}
	return s.Store.GetChat(ctx, id)
}

func (s *erroringStore) AllSegments(ctx context.Context) ([]domain.AgentSegment, error) {
	if s.failAllSegments {
		return nil, fmt.Errorf("boom: all segments")
	}
	return s.Store.AllSegments(ctx)
}

type testFixture struct {
	usecase *agentusecase.Usecase
	repo    agentchat.Store
	term    *fakeCommander
	bc      *fakeBroadcaster
	ws      *fakeWorkspace
}

func newFixture(t *testing.T) testFixture {
	t.Helper()
	return newFixtureWithRepo(t, newRealStore(t))
}

// newFixtureWithRepo builds a fixture over a caller-supplied Store, so a test
// can wrap the real store in an erroringStore to force a specific persistence
// call to fail.
func newFixtureWithRepo(t *testing.T, repo agentchat.Store) testFixture {
	t.Helper()
	t.Setenv("CROWBAR_HOOK_BIN", "/fake/bin/crowbar")

	term := &fakeCommander{}
	bc := &fakeBroadcaster{}
	ws := &fakeWorkspace{
		home:      t.TempDir(),
		projectID: "p1",
		repoID:    "r1",
		worktree:  t.TempDir(),
	}

	u := agentusecase.New(repo, engineagent.NewRegistry(), term, bc, ws)
	return testFixture{usecase: u, repo: repo, term: term, bc: bc, ws: ws}
}

func newRealStore(t *testing.T) agentchat.Store {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "v.db")
	db, err := storesqlite.OpenDB(dbPath)
	require.NoError(t, err)
	repo, err := agentchat.New(db)
	require.NoError(t, err)
	return repo
}

func TestSpawnChat_PersistsChatAndSegmentAndSpawns(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, segID)

	chat, err := f.repo.GetChat(ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, "ws1", chat.WorkspaceID)
	assert.Equal(t, segID, chat.ActiveSegmentID)

	seg, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, chatID, seg.ChatID)
	assert.Equal(t, "claude", seg.ProviderID)
	assert.Equal(t, segID, seg.CrowbarSegmentID)
	assert.Equal(t, "active", seg.Status)
	assert.NotEmpty(t, seg.TerminalSessionID)

	require.Len(t, f.term.calls, 1)
	call := f.term.calls[0]
	assert.Equal(t, "ws1", call.workspaceID)
	assert.Equal(t, f.ws.worktree, call.cwd)
	assert.Equal(t, "claude", call.argv[0])
	// A fresh SpawnChat injects the title instruction via the descriptor's
	// handoff_inject mechanism (see TestSpawnChat_InjectsTitleInstruction for
	// content assertions); it must be present here too, not the raw ledger
	// handoff (there is none yet for a brand-new chat).
	assert.Contains(t, call.argv, "--append-system-prompt")
}

// TestSpawnSegment_TmpDirSurvivesSpawnAndIsRemovedOnlyWhenSessionEnds guards
// the resource-leak fix: spawnSegment's per-spawn tmp dir (holding the
// rendered hook config, and for codex a copy of ~/.codex/auth.json) must be
// home-scoped under <home>/agent-tmp/<segID>, must still exist right after
// spawn (the running CLI reads it for its whole lifetime), and must be
// removed only when the terminal engine invokes the onExit callback passed to
// CreateCommand — i.e. when the CLI's PTY session actually ends, not eagerly
// after the spawn call returns.
func TestSpawnSegment_TmpDirSurvivesSpawnAndIsRemovedOnlyWhenSessionEnds(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	tmpDir := filepath.Join(f.ws.home, "agent-tmp", segID)
	info, err := os.Stat(tmpDir)
	require.NoError(t, err, "agent-tmp dir must exist immediately after spawn")
	assert.True(t, info.IsDir())

	require.Len(t, f.term.calls, 1)
	require.NotNil(t, f.term.calls[0].onExit, "CreateCommand must receive a non-nil onExit")

	// Session still "running": tmp dir must not have been touched.
	_, err = os.Stat(tmpDir)
	require.NoError(t, err, "agent-tmp dir must survive while the CLI is running")

	// Simulate the terminal engine firing onExit once the PTY session ends.
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

			require.Len(t, f.term.calls, 1)
			assert.Equal(t, providerID, f.term.calls[0].argv[0])
		})
	}
}

func TestSpawnSegment_CrowbarHookPathFallsBackToHomeBinCrowbar(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "v.db")
	db, err := storesqlite.OpenDB(dbPath)
	require.NoError(t, err)
	repo, err := agentchat.New(db)
	require.NoError(t, err)

	t.Setenv("CROWBAR_HOOK_BIN", "")

	term := &fakeCommander{}
	home := t.TempDir()
	ws := &fakeWorkspace{home: home, projectID: "p1", repoID: "r1", worktree: t.TempDir()}
	u := agentusecase.New(repo, engineagent.NewRegistry(), term, &fakeBroadcaster{}, ws)
	ctx := context.Background()

	_, _, err = u.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.Len(t, term.calls, 1)
	settingsPath := argAfter(t, term.calls[0].argv, "--settings")
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
// attribution fix: the segment id and provider that identify which chat/CLI a
// hook came from are now rendered into the hook config command line via the
// descriptor's {segid}/{provider} template vars, not injected as an
// environment variable — so a hook can self-identify without trusting an env
// var the spawned CLI could otherwise see/tamper with.
func TestSpawn_HookConfigCarriesSegmentAndProvider(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.Len(t, f.term.calls, 1)
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

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-abc",
	}))
	require.NoError(t, err)

	seg, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "sid-abc", seg.ProviderSessionID)

	require.Len(t, f.bc.calls, 1)
	assert.Equal(t, "bound", f.bc.calls[0].kind)
}

func TestIngestHook_SessionStart_Bound_NeverOverwritesExistingProviderSessionID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	seg, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	seg.ProviderSessionID = "sid-preexisting"
	require.NoError(t, f.repo.SaveSegment(ctx, seg))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-new",
	}))
	require.NoError(t, err)

	got, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "sid-preexisting", got.ProviderSessionID)
}

func TestIngestHook_SessionStart_SameSessionIsNoop(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	require.Len(t, f.bc.calls, 2)
	assert.Equal(t, "bound", f.bc.calls[0].kind)
	assert.Equal(t, "noop", f.bc.calls[1].kind)

	seg, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "sid-1", seg.ProviderSessionID)
}

func TestIngestHook_SessionStart_Registered_MovesOldSegmentAndCreatesNewChat(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-1",
	})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{
		"session_id": "sid-2",
	})))

	oldSeg, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "moved", oldSeg.Status)
	assert.Equal(t, "sid-1", oldSeg.ProviderSessionID)
	require.NotNil(t, oldSeg.EndedAt)

	newActive, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)
	assert.NotEqual(t, segID, newActive.ID)
	assert.Equal(t, "sid-2", newActive.ProviderSessionID)
	assert.NotEqual(t, chatID, newActive.ChatID)
	assert.Equal(t, oldSeg.TerminalSessionID, newActive.TerminalSessionID)

	newChat, err := f.repo.GetChat(ctx, newActive.ChatID)
	require.NoError(t, err)
	assert.Equal(t, "ws1", newChat.WorkspaceID)
	assert.Equal(t, newActive.ID, newChat.ActiveSegmentID)

	require.Len(t, f.bc.calls, 2)
	assert.Equal(t, "bound", f.bc.calls[0].kind)
	assert.Equal(t, "registered", f.bc.calls[1].kind)
	assert.Equal(t, newActive.ChatID, f.bc.calls[1].chatID)
}

// TestIngestHook_SessionStart_Registered_ClearsVacatedChatsActiveSegmentID
// guards the stale-ActiveSegmentID fix: once oldSeg moves away from its
// original chat into a brand-new one, the vacated chat's ActiveSegmentID must
// no longer point at the now-"moved" segment.
func TestIngestHook_SessionStart_Registered_ClearsVacatedChatsActiveSegmentID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	vacatedChat, err := f.repo.GetChat(ctx, chatID)
	require.NoError(t, err)
	assert.Empty(t, vacatedChat.ActiveSegmentID, "vacated chat's ActiveSegmentID must be cleared, not left pointing at the moved segment")
}

func TestIngestHook_SessionStart_Focus_ReactivatesKnownChat(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	active, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, chatID, active.ChatID)
	assert.Equal(t, "sid-1", active.ProviderSessionID)

	chat, err := f.repo.GetChat(ctx, chatID)
	require.NoError(t, err)
	assert.Equal(t, active.ID, chat.ActiveSegmentID)

	require.Len(t, f.bc.calls, 3)
	assert.Equal(t, "focus", f.bc.calls[2].kind)
	assert.Equal(t, chatID, f.bc.calls[2].chatID)
}

// TestIngestHook_SessionStart_Focus_ClearsVacatedChatsActiveSegmentID guards
// the same stale-ActiveSegmentID fix on the "focus" outcome: sid-2's chat
// (registered in the middle step) loses its live segment when the process
// focuses back to sid-1's chat, so its ActiveSegmentID must be cleared too.
func TestIngestHook_SessionStart_Focus_ClearsVacatedChatsActiveSegmentID(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	registeredActive, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)
	registeredChatID := registeredActive.ChatID

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	vacatedChat, err := f.repo.GetChat(ctx, registeredChatID)
	require.NoError(t, err)
	assert.Empty(t, vacatedChat.ActiveSegmentID, "vacated chat's ActiveSegmentID must be cleared after focus moves away from it")
}

// TestIngestHook_TurnStopAppendsAssistantTurn guards the hook-derived-turn
// fix: turn_stop no longer reads a vendor transcript file off disk — the
// assistant's turn text comes straight from the hook payload's
// last_assistant_message field (via the descriptor's canonical "message"
// mapping) and is appended to the ledger directly.
func TestIngestHook_TurnStopAppendsAssistantTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "s1", "last_assistant_message": "done thing"}))
	require.NoError(t, err)

	handoff, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "assistant (claude): done thing")

	require.Len(t, f.bc.calls, 1)
	assert.Equal(t, "turn_stopped", f.bc.calls[0].kind)
	assert.Equal(t, chatID, f.bc.calls[0].chatID)
}

// TestIngestHook_UserPromptAppendsUserTurn guards the new user_prompt
// canonical event: it appends a "user" turn to the ledger from the hook
// payload's prompt field.
func TestIngestHook_UserPromptAppendsUserTurn(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "claude", "user_prompt",
		mustJSON(t, map[string]any{"prompt": "please do the thing"}))
	require.NoError(t, err)

	handoff, err := f.usecase.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "user: please do the thing")

	// The user_prompt hook also fires the derived-title fallback (an empty
	// title picks up the prompt's first line), so "titled" broadcasts before
	// "user_prompt".
	require.Len(t, f.bc.calls, 2)
	assert.Equal(t, "titled", f.bc.calls[0].kind)
	assert.Equal(t, chatID, f.bc.calls[0].chatID)
	assert.Equal(t, "user_prompt", f.bc.calls[1].kind)
	assert.Equal(t, chatID, f.bc.calls[1].chatID)
}

// TestIngestHook_TurnStop_EmptyMessage_NoOps guards appendTurn's empty-text
// no-op: a turn_stop hook whose payload carries no last_assistant_message
// (e.g. a turn that produced no final assistant text) must not write a
// ledger entry or broadcast a lifecycle event.
func TestIngestHook_TurnStop_EmptyMessage_NoOps(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "claude", "turn_stop",
		mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.NoError(t, err)
	assert.Empty(t, f.bc.calls)
}

func TestIngestHook_TurnStop_AfterMove_AttributesToNewChat(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	newActive, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "turn_stop", mustJSON(t, map[string]any{
		"session_id":             "sid-2",
		"last_assistant_message": "second chat transcript",
	})))

	handoff, err := f.usecase.AssembleHandoff(ctx, newActive.ChatID)
	require.NoError(t, err)
	assert.Contains(t, handoff, "second chat transcript")
}

func TestIngestHook_UnknownSegment_IsIgnored(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.IngestHook(ctx, "does-not-exist", "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.NoError(t, err)
	assert.Empty(t, f.bc.calls)
}

func TestIngestHook_UnmappedCanonicalEvent_ReturnsNil(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "claude", "not_a_real_hook", mustJSON(t, map[string]any{}))
	require.NoError(t, err)
	assert.Empty(t, f.bc.calls)
}

func TestSeedRegistry_RehydratesKnownSessions(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	require.NoError(t, f.repo.SaveChat(ctx, domain.AgentChat{ID: "cK", WorkspaceID: "ws1", CreatedAt: time.Now()}))
	require.NoError(t, f.repo.SaveSegment(ctx, domain.AgentSegment{
		ID:                "sK",
		ChatID:            "cK",
		ProviderID:        "claude",
		CrowbarSegmentID:  "sK",
		ProviderSessionID: "sid-known",
		Status:            "ended",
		StartedAt:         time.Now(),
	}))
	require.NoError(t, f.repo.SaveSegment(ctx, domain.AgentSegment{
		ID:               "sX",
		ChatID:           "cK",
		ProviderID:       "claude",
		CrowbarSegmentID: "sX",
		Status:           "ended",
		StartedAt:        time.Now(),
	}))

	require.NoError(t, f.usecase.SeedRegistry(ctx))

	_, segA, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segA, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "s0"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segA, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-known"})))

	active, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segA)
	require.NoError(t, err)
	assert.Equal(t, "cK", active.ChatID)
}

func TestListChatsGetChatSegmentsFor(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

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

func TestSpawnChat_SaveChatFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveChatAt: 1}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spawn chat: save chat")
}

func TestSpawnChat_SaveSegmentFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 1}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spawn segment: save segment")
}

func TestSpawnChat_SaveTerminalSessionIDFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 2}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save terminal session id")
}

func TestSpawnChat_SaveChatActiveSegmentFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveChatAt: 2}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, _, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save chat active segment")
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
	f := newFixture(t)
	ctx := context.Background()

	require.NoError(t, f.repo.SaveSegment(ctx, domain.AgentSegment{
		ID:               "s1",
		ChatID:           "missing-chat",
		ProviderID:       "claude",
		CrowbarSegmentID: "seg1",
		Status:           "active",
		StartedAt:        time.Now(),
	}))

	err := f.usecase.IngestHook(ctx, "seg1", "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ingest hook: chat")
}

func TestIngestHook_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	f.ws.err = fmt.Errorf("boom: worktree lookup")
	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "worktree dir")
}

func TestIngestHook_UnknownProvider_ReturnsWrappedDescriptorError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	require.NoError(t, f.repo.SaveChat(ctx, domain.AgentChat{ID: "c1", WorkspaceID: "ws1", CreatedAt: time.Now()}))
	require.NoError(t, f.repo.SaveSegment(ctx, domain.AgentSegment{
		ID:               "s1",
		ChatID:           "c1",
		ProviderID:       "not-a-real-provider",
		CrowbarSegmentID: "seg1",
		Status:           "active",
		StartedAt:        time.Now(),
	}))

	err := f.usecase.IngestHook(ctx, "seg1", "not-a-real-provider", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve descriptor")
}

// The "registered" and "focus" persistence helpers each make several ordered
// Save*/Get* calls against the repo; these tests target one call at a time
// (by 1-indexed position across the whole SpawnChat+IngestHook sequence) to
// exercise each guard clause's error-wrap branch individually.

func TestIngestHook_Registered_OldSegmentSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 4}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registered: save old segment")
}

func TestIngestHook_Registered_NewSegmentSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 5}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registered: save new segment")
}

func TestIngestHook_Registered_NewChatSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveChatAt: 3}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registered: save new chat")
}

func TestIngestHook_Focus_OldSegmentSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 6}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "focus: save old segment")
}

func TestIngestHook_Focus_NewSegmentSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 7}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "focus: save new segment")
}

func TestIngestHook_Focus_ChatLoadFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failGetChatAt: 4}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "focus: load chat")
}

// TestIngestHook_Registered_ClearVacatedChatFailure_ReturnsWrappedError
// targets the NEW SaveChat call persistRegistered makes to clear the vacated
// chat's ActiveSegmentID: SpawnChat makes 2 SaveChat calls (create + set
// active), the first session_start's "bound" outcome makes none, and
// "registered" makes one more (the new chat) before this one — so the 4th
// SaveChat call is the vacated-chat clear.
func TestIngestHook_Registered_ClearVacatedChatFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveChatAt: 4}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registered: clear vacated chat")
}

// TestIngestHook_Focus_ClearVacatedChatGetFailure_ReturnsWrappedError and
// TestIngestHook_Focus_ClearVacatedChatSaveFailure_ReturnsWrappedError target
// clearVacatedChatActiveSegment's own Get/Save calls in the focus path (the
// 5th GetChat call and 6th SaveChat call across the 3-session_start
// bound->registered->focus sequence, both AFTER the existing focus:load
// chat/focus:save chat calls already covered by other tests here).
func TestIngestHook_Focus_ClearVacatedChatGetFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failGetChatAt: 5}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "focus: clear vacated chat")
}

func TestIngestHook_Focus_ClearVacatedChatSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveChatAt: 6}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"})))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-2"})))

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "focus: clear vacated chat")
}

func TestIngestHook_Bound_SaveSegmentFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 3}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "claude", "session_start", mustJSON(t, map[string]any{"session_id": "sid-1"}))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bound: save segment")
}

func TestSeedRegistry_AllSegmentsFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failAllSegments: true}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	err := f.usecase.SeedRegistry(ctx)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "seed registry")
}
