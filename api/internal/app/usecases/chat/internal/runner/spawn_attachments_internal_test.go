package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// stubWorkspaceForSpawn answers the two calls spawnPaths makes — WorktreeDir
// and AgentChatsDir — from fixed dirs under one temp home.
type stubWorkspaceForSpawn struct {
	seam.WorkspaceReader
	home, worktree, chatsDir string
}

func (s stubWorkspaceForSpawn) WorktreeDir(
	context.Context, string,
) (string, string, string, string, error) {
	return s.home, "p1", "r1", s.worktree, nil
}

func (s stubWorkspaceForSpawn) AgentChatsDir(
	context.Context, string,
) (string, error) {
	return s.chatsDir, nil
}

// stubProvidersForSpawn always enables the provider and never turns its MCP
// surface on — this test never touches tool injection.
type stubProvidersForSpawn struct{}

func (stubProvidersForSpawn) RequireProviderEnabled(context.Context, string) error { return nil }

func (stubProvidersForSpawn) ProviderMCPEnabled(context.Context, string) (bool, error) {
	return false, nil
}

// stubConversationsForSpawn answers only the two calls a create=true spawn's
// spawnPreflight makes — ThreadContext and ChatSelection — panicking on
// anything else via the embedded nil interface, the same pattern
// stubRunnerStoreForResumable uses in switch_internal_test.go.
type stubConversationsForSpawn struct {
	Conversations
}

func (stubConversationsForSpawn) ThreadContext(context.Context, string, bool) (string, error) {
	return "", nil
}

func (stubConversationsForSpawn) ChatSelection(
	context.Context, string, bool,
) (engineagents.Selection, error) {
	return engineagents.Selection{}, nil
}

// boomSpawnPlanDescriptor is apiTransportTestDescriptor's own shape (see
// apiconn_internal_test.go) plus a spawn.args flag that also names itself in
// forbid_flags. Descriptor validation never cross-checks spawn.args against
// forbid_flags (only catalog/telemetry commands — see spawn_command.go and
// catalog_command.go), so this loads cleanly and fails only later, inside
// spawn.Plan's own checkForbidden — the one reachable way to make a REAL
// descriptor's SpawnPlan fail without hand-faking the whole Agent interface.
const boomSpawnPlanDescriptor = `
id: boom-test
spawn:
  cmd: acme
  interactive_required: true
  args: ["--boom"]
  forbid_flags: ["--boom"]
events:
  session_start:
    in: thread/started
    map: { session_id: thread.id }
  turn_stop:
    in: turn/completed
    map:
      session_id: threadId
      message: "turn.items[type=agentMessage].text"
runtime:
  transport: api
  api:
    protocol: jsonrpc2
    serve: [acme, serve]
    handshake: { call: initialize }
`

// TestSpawnRunner_SpawnPlanFailure_CleansUpMaterializedAttachments pins the
// defensive cleanup on spawnRunner's one abort path between materialization
// and fork: a descriptor whose SpawnPlan itself fails must not leak the
// scratch attachment copy materialized moments earlier for dispatch.
func TestSpawnRunner_SpawnPlanFailure_CleansUpMaterializedAttachments(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "boom-test.yaml"), []byte(boomSpawnPlanDescriptor), 0o600))

	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	rs := &Runners{
		agents:        engineagents.New(),
		ws:            stubWorkspaceForSpawn{home: home, worktree: worktree, chatsDir: chatsDir},
		providers:     stubProvidersForSpawn{},
		conversations: stubConversationsForSpawn{},
	}

	text := "![photo](chats/chat-1/attachments/" + fileName + ")"

	// Non-vacuousness checkpoint: prove materialization for this exact input
	// actually produces the scratch file, BEFORE spawnRunner's own internal
	// call (which redoes the same, idempotent write) runs into the induced
	// SpawnPlan failure and reaps it — otherwise the assertion below would
	// pass just as well against a materialize call that silently did nothing.
	scratchFile := filepath.Join(worktreepath.AttachmentScratchDir(worktree, "runner-1"), fileName)
	_, preErr := materializeAttachmentsForDispatch(chatsDir, worktree, "chat-1", "runner-1", text)
	require.NoError(t, preErr)
	require.FileExists(t, scratchFile)

	_, err = rs.spawnRunner(context.Background(), "chat-1", "ws-1", "boom-test", "runner-1",
		nil, nil, "", 0, false, "", true, text)

	require.Error(t, err)
	assert.ErrorContains(t, err, "build spawn plan")

	scratchDir := worktreepath.AttachmentScratchDir(worktree, "runner-1")
	_, statErr := os.Stat(scratchDir)
	assert.True(t, os.IsNotExist(statErr),
		"the scratch attachment dir materialized before the failed SpawnPlan must be reaped")
}

// TestSpawnRunner_DescriptorResolveFailure_CleansUpMaterializedAttachments
// pins the fix for spawnRunner's descriptor-resolution branch, which — unlike
// every other post-materialization abort path in this function (materialize
// failure above, SpawnPlan failure above, forkCLI's own branches, onExit) —
// omitted the scratch-dir cleanup: rs.agents.Get failing for an id with no
// matching descriptor file must not leak the scratch copy materialized
// moments earlier for dispatch. recordRunner never runs on this path, so
// nothing else — not even the boot-time orphan reaper, which only inspects
// runner rows that exist — will ever reap this directory otherwise.
func TestSpawnRunner_DescriptorResolveFailure_CleansUpMaterializedAttachments(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	// Deliberately no descriptor file for "missing-test" — rs.agents.Get must
	// fail to resolve it.

	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	rs := &Runners{
		agents:        engineagents.New(),
		ws:            stubWorkspaceForSpawn{home: home, worktree: worktree, chatsDir: chatsDir},
		providers:     stubProvidersForSpawn{},
		conversations: stubConversationsForSpawn{},
	}

	text := "![photo](chats/chat-1/attachments/" + fileName + ")"
	_, err = rs.spawnRunner(context.Background(), "chat-1", "ws-1", "missing-test", "runner-1",
		nil, nil, "", 0, false, "", true, text)

	require.Error(t, err)
	assert.ErrorContains(t, err, "resolve descriptor")

	scratchDir := worktreepath.AttachmentScratchDir(worktree, "runner-1")
	_, statErr := os.Stat(scratchDir)
	assert.True(t, os.IsNotExist(statErr),
		"the scratch attachment dir materialized before the failed descriptor resolution must be reaped")
}

// okHooksDescriptor is a minimal, entirely valid hooks-transport descriptor —
// unlike boomSpawnPlanDescriptor, its SpawnPlan succeeds — used by the two
// tests below that need to reach forkCLI with a real, working descriptor.
// hooks (not api) transport so this never touches applyAPITransport's
// serve-process machinery at all.
const okHooksDescriptor = `
id: ok-test
spawn:
  cmd: acme
  interactive_required: true
events:
  session_start:
    in: SessionStart
    map: { session_id: session_id }
  turn_stop:
    in: Stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
    delivery: http
`

// TestSpawnRunner_MaterializeFailure_CleansUpTheScratchDir pins the fix for
// the leak materializeAttachmentsForDispatch's OWN error branch left open: a
// write failure after MkdirAll has already created the scratch dir (and
// possibly written some files) must not leave that dir behind — nothing else
// ever reaps it, since the CLI is never forked and onRunnerExit never runs.
func TestSpawnRunner_MaterializeFailure_CleansUpTheScratchDir(t *testing.T) {
	home := t.TempDir()
	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	// Pre-create the destination AS A DIRECTORY, so materializeAttachmentsForDispatch's
	// os.MkdirAll(scratchDir) succeeds (it already exists) but its later
	// os.WriteFile(dest, ...) fails — the exact "MkdirAll already succeeded,
	// WriteFile fails" ordering the reviewer flagged.
	scratchDir := worktreepath.AttachmentScratchDir(worktree, "runner-1")
	require.NoError(t, os.MkdirAll(filepath.Join(scratchDir, fileName), 0o700))

	rs := &Runners{
		ws:            stubWorkspaceForSpawn{home: home, worktree: worktree, chatsDir: chatsDir},
		providers:     stubProvidersForSpawn{},
		conversations: stubConversationsForSpawn{},
	}

	text := "![photo](chats/chat-1/attachments/" + fileName + ")"
	_, err = rs.spawnRunner(context.Background(), "chat-1", "ws-1", "irrelevant-provider", "runner-1",
		nil, nil, "", 0, false, "", true, text)

	require.Error(t, err)
	assert.ErrorContains(t, err, "materialize attachments")

	_, statErr := os.Stat(scratchDir)
	assert.True(t, os.IsNotExist(statErr),
		"the scratch dir must be reaped even when materialization itself is what failed")
}

// TestSpawnRunner_PendingHooksRegisterFailure_CleansUpMaterializedAttachments
// pins the fix for forkCLI's pendingHooks.Register failure branch, which
// omitted the same scratch-dir cleanup its sibling CreateCommand-failure
// branch got: a runner id already registered (a real, reachable duplicate
// condition per pending.Hooks.Register's own doc comment) must not leak the
// scratch copy materialized earlier in this same spawn.
func TestSpawnRunner_PendingHooksRegisterFailure_CleansUpMaterializedAttachments(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "ok-test.yaml"), []byte(okHooksDescriptor), 0o600))

	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")
	durableDir := worktreepath.AttachmentsDir(chatsDir, "chat-1")
	fileName, _, err := repoattachments.Store(durableDir, "ab12", "photo.png", []byte("bytes"))
	require.NoError(t, err)

	pendingHooks := inflight.NewHooks()
	// Pre-claim the runner id spawnRunner is about to use, so forkCLI's own
	// Register call fails exactly the way a genuine duplicate registration
	// would.
	require.NoError(t, pendingHooks.Register("runner-1"))

	rs := &Runners{
		agents:        engineagents.New(),
		ws:            stubWorkspaceForSpawn{home: home, worktree: worktree, chatsDir: chatsDir},
		providers:     stubProvidersForSpawn{},
		conversations: stubConversationsForSpawn{},
		pendingHooks:  pendingHooks,
	}

	text := "![photo](chats/chat-1/attachments/" + fileName + ")"
	_, err = rs.spawnRunner(context.Background(), "chat-1", "ws-1", "ok-test", "runner-1",
		nil, nil, "", 0, false, "", true, text)

	require.Error(t, err)
	assert.ErrorContains(t, err, "install hook startup barrier")

	scratchDir := worktreepath.AttachmentScratchDir(worktree, "runner-1")
	_, statErr := os.Stat(scratchDir)
	assert.True(t, os.IsNotExist(statErr),
		"the scratch attachment dir materialized before the failed hook barrier registration must be reaped")
}
