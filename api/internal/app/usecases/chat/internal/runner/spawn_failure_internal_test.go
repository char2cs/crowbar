package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/inflight"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
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

func TestSpawnRunner_SpawnPlanFailure_ReturnsTheBuildError(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "boom-test.yaml"), []byte(boomSpawnPlanDescriptor), 0o600))

	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")

	rs := &Runners{
		agents:        engineagents.New(),
		ws:            stubWorkspaceForSpawn{home: home, worktree: worktree, chatsDir: chatsDir},
		providers:     stubProvidersForSpawn{},
		conversations: stubConversationsForSpawn{},
	}

	_, err := rs.spawnRunner(context.Background(), "chat-1", "ws-1", "boom-test", "runner-1",
		nil, nil, "", 0, false, "", true, "hello")

	require.Error(t, err)
	assert.ErrorContains(t, err, "build spawn plan")
}

func TestSpawnRunner_DescriptorResolveFailure_ReturnsTheResolveError(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	// Deliberately no descriptor file for "missing-test" — rs.agents.Get must
	// fail to resolve it.

	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")

	rs := &Runners{
		agents:        engineagents.New(),
		ws:            stubWorkspaceForSpawn{home: home, worktree: worktree, chatsDir: chatsDir},
		providers:     stubProvidersForSpawn{},
		conversations: stubConversationsForSpawn{},
	}

	_, err := rs.spawnRunner(context.Background(), "chat-1", "ws-1", "missing-test", "runner-1",
		nil, nil, "", 0, false, "", true, "hello")

	require.Error(t, err)
	assert.ErrorContains(t, err, "resolve descriptor")
}

// okHooksDescriptor is a minimal, entirely valid hooks-transport descriptor —
// unlike boomSpawnPlanDescriptor, its SpawnPlan succeeds — used by the test
// below that needs to reach forkCLI with a real, working descriptor. hooks
// (not api) transport so this never touches applyAPITransport's serve-process
// machinery at all.
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

func TestSpawnRunner_PendingHooksRegisterFailure_ReturnsTheBarrierError(t *testing.T) {
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(home, "descriptors", "ok-test.yaml"), []byte(okHooksDescriptor), 0o600))

	worktree := filepath.Join(home, "worktree")
	chatsDir := filepath.Join(home, "chats")

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

	_, err := rs.spawnRunner(context.Background(), "chat-1", "ws-1", "ok-test", "runner-1",
		nil, nil, "", 0, false, "", true, "hello")

	require.Error(t, err)
	assert.ErrorContains(t, err, "install hook startup barrier")
}
