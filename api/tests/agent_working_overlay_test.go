//go:build integration

package tests

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// liveStubProviderDescriptorYAML spawns `cat`, which stays alive on its PTY
// (no stdin EOF) so the segment remains active while user_prompt/turn_stop hooks
// fire — unlike the `true` stub, which exits instantly and ends its segment.
const liveStubProviderDescriptorYAML = `id: livestub
spawn:
  cmd: "cat"
  interactive_required: true
hooks:
  format: json
  events:
    session_start: { session_id: session_id }
    user_prompt: { message: prompt }
    turn_stop: { session_id: session_id, message: last_assistant_message }
`

func writeLiveStubProviderDescriptor(t *testing.T, h *harness) {
	t.Helper()
	dir := filepath.Join(h.home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "livestub.yaml"), []byte(liveStubProviderDescriptorYAML), 0o644))
}

// TestRegression_WorkspaceWorkingReflectsAgentTurn proves the workspace `working`
// overlay (domain.Workspace.Working) is re-lit from live agent turns: a
// user_prompt hook (→ turn_started) re-broadcasts the workspace with working=true,
// and a turn_stop hook (→ turn_stopped) re-broadcasts it with working=false.
func TestRegression_WorkspaceWorkingReflectsAgentTurn(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)
	repoBase := "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID

	var created struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/agent/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	h.Quiesce()

	var detail struct {
		Segments []struct {
			CrowbarSegmentID string `json:"crowbarSegmentId"`
		} `json:"segments"`
	}
	h.get(wsBase(imported)+"/agent/chats/"+created.ID, &detail)
	require.NotEmpty(t, detail.Segments)
	segID := detail.Segments[0].CrowbarSegmentID

	conn := h.dial(repoBase + "/workspaces")

	// user_prompt opens the turn.
	_ = h.raw(http.MethodPost, wsBase(imported)+"/agent/hooks", map[string]string{
		"segment_id": segID, "provider": "livestub", "event": "user_prompt",
		"payload_raw": `{"prompt":"hi"}`,
	}, http.StatusAccepted).Body.Close()
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["working"] == true
	})

	// turn_stop closes it.
	_ = h.raw(http.MethodPost, wsBase(imported)+"/agent/hooks", map[string]string{
		"segment_id": segID, "provider": "livestub", "event": "turn_stop",
		"payload_raw": `{"last_assistant_message":"done"}`,
	}, http.StatusAccepted).Body.Close()
	readUntil(t, conn, func(m map[string]any) bool {
		return m["id"] == imported.workspaceID && m["working"] == false
	})
}
