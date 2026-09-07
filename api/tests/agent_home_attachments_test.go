//go:build integration

package tests

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentHomeAttachmentUploadAndRead proves the chat-attachment
// upload/read pair (chat/routes.go's .../chats/:id/attachments and
// .../chats/:id/attachments/:file) is reachable under the project-home mount,
// not only under .../workspaces/:wsId. Those two routes were added to the
// workspace-scoped group only; home/routes.go's registerAgent — the SECOND
// copy of that table TestHomeMountsEveryAgentRoute guards — never got them
// mirrored in, so a project-home (repo-less) workspace 404'd on every
// attachment operation. TestHomeMountsEveryAgentRoute pins the route table;
// this pins the functional path end to end (an upload, then reading the
// stored bytes back through the home mount), the way
// TestRegression_AgentHomeCallbacksReachDaemon does for the hook callbacks.
func TestRegression_AgentHomeAttachmentUploadAndRead(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	// The JSON path-ingestion shape (a revived desktop drag-and-drop) — see
	// UploadAttachment's own doc comment — is the easiest to drive from an
	// HTTP client that doesn't want to hand-build multipart bodies.
	dir := t.TempDir()
	src := filepath.Join(dir, "notes.txt")
	const want = "hello from a home-workspace attachment"
	require.NoError(t, os.WriteFile(src, []byte(want), 0o600))

	var uploaded struct {
		Ref         string `json:"ref"`
		FileName    string `json:"fileName"`
		Size        int    `json:"size"`
		ContentType string `json:"contentType"`
	}
	h.post(homeBase+"/chats/"+created.ID+"/attachments",
		map[string]string{"path": src, "id": "att1"}, http.StatusCreated, &uploaded)
	assert.Equal(t, "att1-notes.txt", uploaded.FileName)
	assert.NotEmpty(t, uploaded.Ref)
	assert.Equal(t, len(want), uploaded.Size)

	resp := h.raw(http.MethodGet,
		homeBase+"/chats/"+created.ID+"/attachments/"+uploaded.FileName, nil, http.StatusOK)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, want, string(body))
}

// TestRegression_AttachmentHeadReportsContentLength proves a HEAD against the
// attachment-read route reports the real file size, not a 404 — the web
// client's own file-card size label (chat-asset-resolver.ts's
// fetchChatAttachmentMetadata) is a HEAD against this exact route, with no
// GET-only route registered here it always 404's, and Gin's own JSON 404
// body's incidental Content-Length was silently displayed as the file's
// size instead. The handler itself (attachments.go's Attachment) never
// checks the request method, so a HEAD route reaching it is enough: Go's
// net/http server suppresses the body but still reports the real
// Content-Length it wrote.
func TestRegression_AttachmentHeadReportsContentLength(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"

	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.Quiesce()

	dir := t.TempDir()
	src := filepath.Join(dir, "report.pdf")
	const want = "hello from a HEAD-requested attachment"
	require.NoError(t, os.WriteFile(src, []byte(want), 0o600))

	var uploaded struct {
		FileName string `json:"fileName"`
	}
	h.post(homeBase+"/chats/"+created.ID+"/attachments",
		map[string]string{"path": src, "id": "att2"}, http.StatusCreated, &uploaded)

	resp := h.raw(http.MethodHead,
		homeBase+"/chats/"+created.ID+"/attachments/"+uploaded.FileName, nil, http.StatusOK)
	defer func() { _ = resp.Body.Close() }()
	assert.EqualValues(t, len(want), resp.ContentLength)
}
