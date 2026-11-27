//go:build integration

package tests

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestAttachmentScriptableTypesDoNotRunOnTheAppOrigin(t *testing.T) {
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
	files := map[string]struct {
		body   string
		inline bool
	}{
		"page.html": {`<!doctype html><html><script>fetch("/v0/projects")</script></html>`, false},
		"logo.svg":  {`<svg xmlns="http://www.w3.org/2000/svg"><script>alert(1)</script></svg>`, false},
		"shot.png":  {"\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR", true},
	}
	for name, f := range files {
		src := filepath.Join(dir, name)
		require.NoError(t, os.WriteFile(src, []byte(f.body), 0o600))
		var uploaded struct {
			FileName string `json:"fileName"`
		}
		h.post(homeBase+"/chats/"+created.ID+"/attachments",
			map[string]string{"path": src, "id": "id-" + name[:4]}, http.StatusCreated, &uploaded)

		resp := h.raw(http.MethodGet,
			homeBase+"/chats/"+created.ID+"/attachments/"+uploaded.FileName, nil, http.StatusOK)
		require.NoError(t, resp.Body.Close())

		assert.Equal(t, "nosniff", resp.Header.Get("X-Content-Type-Options"), name)
		if f.inline {
			assert.Equal(t, "image/png", resp.Header.Get("Content-Type"), name)
			assert.Equal(t, "inline", resp.Header.Get("Content-Disposition"), name)
			continue
		}
		assert.Equal(t, "attachment", resp.Header.Get("Content-Disposition"), name)
		assert.Contains(t, resp.Header.Get("Content-Security-Policy"), "sandbox", name)
	}
}

func TestRegression_ChunkedAttachmentUploadReachesTheHomeMount(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	homeBase := "/v0/projects/" + imported.projectID + "/home"
	var created struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/chats", map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	h.Quiesce()
	uploads := homeBase + "/chats/" + created.ID + "/attachments/uploads"

	const want = "chunked hello"
	var begun struct {
		Received int64 `json:"received"`
	}
	h.post(uploads, map[string]any{"id": "ck1", "fileName": "n.txt", "size": len(want)}, http.StatusOK, &begun)
	assert.Zero(t, begun.Received)
	for _, c := range []struct {
		offset int
		part   string
	}{{0, want[:6]}, {6, want[6:]}} {
		offset, part := c.offset, c.part
		req, err := http.NewRequest(http.MethodPut, fmt.Sprintf("%s%s/ck1?offset=%d", h.url, uploads, offset), strings.NewReader(part))
		require.NoError(t, err)
		resp, err := h.server.Client().Do(req)
		require.NoError(t, err)
		_ = resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)
	}
	var done struct {
		FileName string `json:"fileName"`
		Size     int    `json:"size"`
	}
	h.post(uploads+"/ck1/complete", map[string]any{"fileName": "n.txt", "size": len(want)}, http.StatusCreated, &done)
	assert.Equal(t, "ck1-n.txt", done.FileName)

	resp := h.raw(http.MethodGet, homeBase+"/chats/"+created.ID+"/attachments/"+done.FileName, nil, http.StatusOK)
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	assert.Equal(t, want, string(body))

	h.post(uploads, map[string]any{"id": "ck2", "fileName": "n.txt", "size": 4}, http.StatusOK, &begun)
	h.del(uploads+"/ck2", nil, http.StatusNoContent, nil)
	h.del(uploads+"/ck2", nil, http.StatusNoContent, nil)
}
