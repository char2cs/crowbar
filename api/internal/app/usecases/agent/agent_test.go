package agent_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/agent"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagent "github.com/char2cs/crowbar/api/internal/engine/agent"
)

type commandCall struct {
	workspaceID string
	cwd         string
	argv        []string
	env         []string
}

type fakeCommander struct {
	calls  []commandCall
	killed []string
	nextID int
	err    error
}

func (f *fakeCommander) CreateCommand(
	_ context.Context,
	workspaceID string,
	cwd string,
	argv []string,
	env []string,
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
	})
	return fmt.Sprintf("term-%d", f.nextID), nil
}

func (f *fakeCommander) Kill(
	_ context.Context,
	sessionID string,
) error {
	f.killed = append(f.killed, sessionID)
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
	assert.Contains(t, call.env, "CROWBAR_SEGMENT_ID="+segID)
	assert.NotContains(t, call.argv, "--append-system-prompt")
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

func TestIngestHook_SessionStart_Bound_RecordsProviderSessionAndTranscript(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{
		"session_id":      "sid-abc",
		"transcript_path": "/tmp/whatever.jsonl",
	})
	require.NoError(t, err)

	seg, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "sid-abc", seg.ProviderSessionID)
	assert.Equal(t, "/tmp/whatever.jsonl", seg.TranscriptPath)

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

	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{
		"session_id":      "sid-new",
		"transcript_path": "/tmp/x.jsonl",
	})
	require.NoError(t, err)

	got, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "sid-preexisting", got.ProviderSessionID)
	assert.Equal(t, "/tmp/x.jsonl", got.TranscriptPath)
}

func TestIngestHook_SessionStart_SameSessionIsNoop(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))

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

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{
		"session_id":      "sid-1",
		"transcript_path": "/tmp/a.jsonl",
	}))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{
		"session_id":      "sid-2",
		"transcript_path": "/tmp/b.jsonl",
	}))

	oldSeg, err := f.repo.GetSegment(ctx, segID)
	require.NoError(t, err)
	assert.Equal(t, "moved", oldSeg.Status)
	assert.Equal(t, "sid-1", oldSeg.ProviderSessionID)
	require.NotNil(t, oldSeg.EndedAt)

	newActive, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)
	assert.NotEqual(t, segID, newActive.ID)
	assert.Equal(t, "sid-2", newActive.ProviderSessionID)
	assert.Equal(t, "/tmp/b.jsonl", newActive.TranscriptPath)
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

func TestIngestHook_SessionStart_Focus_ReactivatesKnownChat(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-2"}))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))

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

func TestIngestHook_TurnStop_AppendsLedgerEntry(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	chatID, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	transcriptPath := filepath.Join(t.TempDir(), "transcript.jsonl")
	require.NoError(t, os.WriteFile(transcriptPath, []byte("hello transcript"), 0o600))

	err = f.usecase.IngestHook(ctx, segID, "turn_stop", map[string]any{
		"session_id":      "sid-1",
		"transcript_path": transcriptPath,
	})
	require.NoError(t, err)

	ledgerDir := worktreepath.AgentLedgerDir(f.ws.home, f.ws.projectID, f.ws.repoID, "ws1", chatID)
	entries, err := os.ReadDir(ledgerDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	data, err := os.ReadFile(filepath.Join(ledgerDir, entries[0].Name()))
	require.NoError(t, err)
	assert.Equal(t, "hello transcript", string(data))

	require.Len(t, f.bc.calls, 1)
	assert.Equal(t, "turn_stopped", f.bc.calls[0].kind)
	assert.Equal(t, chatID, f.bc.calls[0].chatID)
}

func TestIngestHook_TurnStop_MissingTranscript_NoOps(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "turn_stop", map[string]any{
		"session_id":      "sid-1",
		"transcript_path": filepath.Join(t.TempDir(), "does-not-exist.jsonl"),
	})
	require.NoError(t, err)
	assert.Empty(t, f.bc.calls)
}

func TestIngestHook_TurnStop_AfterMove_AttributesToNewChat(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-2"}))

	newActive, err := f.repo.GetActiveSegmentByCrowbarID(ctx, segID)
	require.NoError(t, err)

	transcriptPath := filepath.Join(t.TempDir(), "t2.jsonl")
	require.NoError(t, os.WriteFile(transcriptPath, []byte("second chat transcript"), 0o600))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "turn_stop", map[string]any{
		"session_id":      "sid-2",
		"transcript_path": transcriptPath,
	}))

	ledgerDir := worktreepath.AgentLedgerDir(f.ws.home, f.ws.projectID, f.ws.repoID, "ws1", newActive.ChatID)
	entries, err := os.ReadDir(ledgerDir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	data, err := os.ReadFile(filepath.Join(ledgerDir, entries[0].Name()))
	require.NoError(t, err)
	assert.Equal(t, "second chat transcript", string(data))
}

func TestIngestHook_UnknownSegment_IsIgnored(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	err := f.usecase.IngestHook(ctx, "does-not-exist", "session_start", map[string]any{"session_id": "sid-1"})
	require.NoError(t, err)
	assert.Empty(t, f.bc.calls)
}

func TestIngestHook_UnmappedCanonicalEvent_ReturnsNil(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "not_a_real_hook", map[string]any{})
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

	require.NoError(t, f.usecase.IngestHook(ctx, segA, "session_start", map[string]any{"session_id": "s0"}))
	require.NoError(t, f.usecase.IngestHook(ctx, segA, "session_start", map[string]any{"session_id": "sid-known"}))

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

	err := f.usecase.IngestHook(ctx, "seg1", "session_start", map[string]any{"session_id": "sid-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ingest hook: chat")
}

func TestIngestHook_WorktreeDirFailure_ReturnsWrappedError(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	f.ws.err = fmt.Errorf("boom: worktree lookup")
	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"})
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

	err := f.usecase.IngestHook(ctx, "seg1", "session_start", map[string]any{"session_id": "sid-1"})
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
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))

	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registered: save old segment")
}

func TestIngestHook_Registered_NewSegmentSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 5}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))

	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registered: save new segment")
}

func TestIngestHook_Registered_NewChatSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveChatAt: 3}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))

	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-2"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "registered: save new chat")
}

func TestIngestHook_Focus_OldSegmentSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 6}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-2"}))

	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "focus: save old segment")
}

func TestIngestHook_Focus_NewSegmentSaveFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 7}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-2"}))

	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "focus: save new segment")
}

func TestIngestHook_Focus_ChatLoadFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failGetChatAt: 4}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"}))
	require.NoError(t, f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-2"}))

	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "focus: load chat")
}

func TestIngestHook_Bound_SaveSegmentFailure_ReturnsWrappedError(t *testing.T) {
	repo := &erroringStore{Store: newRealStore(t), failSaveSegAt: 3}
	f := newFixtureWithRepo(t, repo)
	ctx := context.Background()

	_, segID, err := f.usecase.SpawnChat(ctx, "ws1", "claude")
	require.NoError(t, err)

	err = f.usecase.IngestHook(ctx, segID, "session_start", map[string]any{"session_id": "sid-1"})
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
