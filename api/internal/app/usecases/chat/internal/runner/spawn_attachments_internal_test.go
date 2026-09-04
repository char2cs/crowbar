package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	repoattachments "github.com/char2cs/crowbar/api/internal/app/repositories/chat/attachments"
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
	_, err = rs.spawnRunner(context.Background(), "chat-1", "ws-1", "boom-test", "runner-1",
		nil, nil, "", 0, false, "", true, text)

	require.Error(t, err)
	assert.ErrorContains(t, err, "build spawn plan")

	scratchDir := worktreepath.AttachmentScratchDir(worktree, "runner-1")
	_, statErr := os.Stat(scratchDir)
	assert.True(t, os.IsNotExist(statErr),
		"the scratch attachment dir materialized before the failed SpawnPlan must be reaped")
}
