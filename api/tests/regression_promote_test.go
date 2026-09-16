//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// promoteStubProviderDescriptorYAML is cwdstub with one difference that
// matters here: it prints the last TWO segments of its real working directory
// rather than the whole path.
//
// readMarkedValue stops at the first control byte, and a full temp-dir path
// wraps past the PTY's column width — so two DIFFERENT worktrees under the
// same temp root read back as the same truncated prefix, and an assertion
// that the promoted CLI moved would pass whether it moved or not. Every
// managed worktree ends in a literal "worktree" leaf under a directory named
// for its BRANCH (worktreepath.WorkspaceRoot), so the branch segment is
// exactly the part that differs and the pair is short enough to survive the
// wrap.
const promoteStubProviderDescriptorYAML = `id: promotestub
spawn:
  cmd: "sh"
  args: ["-c", "echo CROWBAR_CWD:$(basename $(dirname $(pwd)))/$(basename $(pwd)); exec cat"]
  interactive_required: true
session:
  resume: { arg: "--resume {id}" }
events:
  session_start:
    in: session_start
    map:
      session_id: session_id
  user_prompt:
    in: user_prompt
    map:
      message: prompt
  turn_stop:
    in: turn_stop
    map:
      session_id: session_id
      message: last_assistant_message
runtime:
  transport: hooks
  hooks:
    format: json
`

func writePromoteStubProviderDescriptor(t *testing.T, h *harness) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(dir, "promotestub.yaml"), []byte(promoteStubProviderDescriptorYAML), 0o644))
}

// TestRegression_PromoteFillsABubblesWorkspaceSlotOverHTTP is the route half of
// the model spec's §4.2 promotion: POST /repos/:rid/chats/:id/promote. The
// usecase existed through the whole 23-task plan with nothing mounted on it, so
// the one verb that turns a bubble into a worktree chat was unreachable from
// the product.
//
// It asserts the promotion end to end rather than the response envelope: the
// row keeps its id, gains a REAL workspace, and its CLI is respawned in that
// workspace's own worktree — read back as the OS-level working directory the
// spawned shell itself printed (readSpawnCwd), which is the one fact no
// Crowbar-reported field can fake. A promotion that filled the slot but left
// the CLI in the ancestor's directory would pass every field check and be
// exactly the failure this verb exists to prevent.
func TestRegression_PromoteFillsABubblesWorkspaceSlotOverHTTP(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	ancestorID := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, "")
	ancestorCwd := readSpawnCwd(t, h, imported, ancestorID)
	require.NotEmpty(t, ancestorCwd, "the ancestor's own spawn must report a real cwd")

	// A bubble can only be nested under a FOLDER today, not directly under a
	// workspace-owning chat — checkParentKind's cross-workspace guard has no
	// exemption for a workspace-less child. Same construction
	// TestRegression_BubbleChatSpawnsInAncestorWorktree settled for, and for the
	// same disclosed reason.
	folder := createChatFolder(t, h, base, "promote-holder", ancestorID)
	bubbleID := createChatWithProvider(t, h, base, "promotestub", "", folder.ID)

	before := getAgentChat(t, h, base, bubbleID)
	require.Empty(t, before.WorkspaceID, "the chat under promotion must actually be a bubble")
	require.NotEmpty(t, before.LiveRunnerID, "and it must be live, so there is a provider to respawn")

	var promoted agentChatDetail
	h.post(base+"/chats/"+bubbleID+"/promote", nil, http.StatusOK, &promoted)
	h.QuiesceReactors()

	assert.Equal(t, bubbleID, promoted.ID, "promotion keeps the chat's id")
	require.NotEmpty(t, promoted.WorkspaceID, "promotion must fill the workspace slot")
	assert.Equal(t, before.Title, promoted.Title, "promotion keeps the chat's title")
	assert.Equal(t, folder.ID, promoted.ParentID, "promotion keeps the chat where it sits in the tree")

	detail := getAgentChat(t, h, base, bubbleID)
	require.Equal(t, promoted.WorkspaceID, detail.WorkspaceID,
		"the filled slot is durable, not a fact only the response knew")
	require.NotEmpty(t, detail.LiveRunnerID,
		"the promoted chat must carry a live runner — the respawn is the whole second half of the verb")
	assert.NotEqual(t, before.LiveRunnerID, detail.LiveRunnerID,
		"the respawn places a NEW runner: the old CLI was quit and a new one started in the new worktree")

	promotedCwd := readSpawnCwd(t, h, imported, bubbleID)
	require.NotEqual(t, ancestorCwd, promotedCwd,
		"the promoted chat's CLI must run in its OWN new worktree, not the ancestor's")
}

// TestRegression_PromoteRefusesAChatThatAlreadyOwnsAWorktree pins the route's
// refusal half, and with it the one-way rule: a worktree is never demoted
// (model spec §4.2), so promotion fills an empty slot exactly once. It is a 409
// and not a 500 — the request is well formed, the row is simply not in a state
// where this can be done.
func TestRegression_PromoteRefusesAChatThatAlreadyOwnsAWorktree(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	base := repoBase(imported)

	anchored := createChatWithProvider(t, h, base, "promotestub", imported.workspaceID, "")

	h.postError(base+"/chats/"+anchored+"/promote", nil, http.StatusConflict)

	detail := getAgentChat(t, h, base, anchored)
	assert.Equal(t, imported.workspaceID, detail.WorkspaceID,
		"a refused promotion changes nothing about the row")
}
