//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

func createProjectForHome(t *testing.T, h *harness) string {
	t.Helper()
	dir := gitRepoWithCommit(t)

	projectsWS := watchEntities(t, h, stream.KindProject)
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "home-demo", "path": dir}, http.StatusAccepted)
	_ = resp.Body.Close()

	project := projectsWS.read(t, func(m map[string]any) bool {
		return m["path"] == dir
	})
	id, _ := project["id"].(string)
	require.NotEmpty(t, id, "project create must broadcast a ProjectDTO with an id")
	return id
}

func homeWorkspaceID(t *testing.T, h *harness, projectID string) string {
	t.Helper()
	var home struct {
		ID string `json:"id"`
	}
	h.get("/v0/projects/"+projectID+"/home", &home)
	require.NotEmpty(t, home.ID, "the home endpoint must name its workspace")
	return home.ID
}

func TestRegression_HomeEndpointReachable(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)

	var homeWS struct {
		Kind string `json:"kind"`
	}
	h.get("/v0/projects/"+projectID+"/home", &homeWS)
	require.Equal(t, "home", homeWS.Kind,
		"home endpoint must return a workspace with kind=home")
}

func TestHome_ThreadsRoundTrip(t *testing.T) {
	h := newHarness(t)
	homeBase := "/v0/projects/" + createProjectForHome(t, h) + "/home"

	var opened threadDTO
	h.post(homeBase+"/threads", map[string]any{
		"filePath": "README.md", "line": 3, "side": "right", "body": "home thread",
	}, http.StatusCreated, &opened)
	require.NotEmpty(t, opened.ID, "opening a home thread must return an id")
	assert.Equal(t, "home thread", opened.Body)

	var listed []threadDTO
	h.get(homeBase+"/threads", &listed)
	require.Len(t, listed, 1, "the opened thread must list under the home workspace")
	assert.Equal(t, opened.ID, listed[0].ID)
}

func TestHome_AThreadReachesTheHomeWorkspaceScope(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)
	homeBase := "/v0/projects/" + projectID + "/home"
	scope := "ws:" + homeWorkspaceID(t, h, projectID)

	c := dialStream(t, h)
	c.Hold(t, scope)
	h.post(homeBase+"/threads", map[string]any{
		"filePath": "README.md", "line": 1, "side": "right", "body": "ws home thread",
	}, http.StatusCreated, nil)

	f := c.Next(t, func(f stream.Frame) bool {
		return f.T == stream.FrameEntity && f.Scope == scope && f.Kind == string(stream.KindReviewThread)
	})
	var th threadDTO
	require.NoError(t, json.Unmarshal(f.Data, &th))
	assert.Equal(t, "ws home thread", th.Body)
	assert.Equal(t, projectID, th.ProjectID, "home thread frame must carry the project scope")
}

func TestHome_FilesScopeHoldsTheHomeWorkspace(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)

	dialStream(t, h).Hold(t, "files:"+homeWorkspaceID(t, h, projectID))
}

func TestRegression_HomeHasNoGitSurface(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)

	resp := h.raw(http.MethodGet,
		"/v0/projects/"+projectID+"/home/git/status", nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

func TestRegression_HomeFilesCopy(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)
	homeBase := "/v0/projects/" + projectID + "/home"
	homeID := homeWorkspaceID(t, h, projectID)

	var copied struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/files/copy",
		map[string]string{"sourcePath": "README.md", "destPath": "README copy.md"},
		http.StatusCreated, &copied)
	assert.Equal(t, "README copy.md", copied.ID)

	var original, dup struct {
		Content string `json:"content"`
	}
	h.get(docURL(homeID, "README.md"), &original)
	h.get(docURL(homeID, "README copy.md"), &dup)
	assert.Equal(t, original.Content, dup.Content,
		"home copy must reproduce the source content byte-for-byte")
}

func TestRegression_HomeFilesCopyConflictAndMissingSource(t *testing.T) {
	h := newHarness(t)
	homeBase := "/v0/projects/" + createProjectForHome(t, h) + "/home"

	var copied struct {
		ID string `json:"id"`
	}
	h.post(homeBase+"/files/copy",
		map[string]string{"sourcePath": "README.md", "destPath": "README copy2.md"},
		http.StatusCreated, &copied)

	h.postError(homeBase+"/files/copy",
		map[string]string{"sourcePath": "README.md", "destPath": "README copy2.md"},
		http.StatusConflict)
	h.postError(homeBase+"/files/copy",
		map[string]string{"sourcePath": "ghost.md", "destPath": "ghost copy.md"},
		http.StatusNotFound)
}

func TestRegression_HomeFilesRenameContract(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)
	homeBase := "/v0/projects/" + projectID + "/home"
	homeID := homeWorkspaceID(t, h, projectID)

	var before struct {
		Content string `json:"content"`
	}
	h.get(docURL(homeID, "README.md"), &before)

	var renamed struct {
		ID string `json:"id"`
	}
	h.patch(homeBase+"/files",
		map[string]string{"path": "README.md", "newPath": "RENAMED.md"}, &renamed)
	assert.Equal(t, "RENAMED.md", renamed.ID)

	var after struct {
		Content string `json:"content"`
	}
	h.get(docURL(homeID, "RENAMED.md"), &after)
	assert.Equal(t, before.Content, after.Content, "rename must preserve content at the new path")
	require.NoError(t, h.raw(http.MethodGet, docURL(homeID, "README.md"), nil, http.StatusNotFound).Body.Close())

	h.raw(http.MethodPatch, homeBase+"/files",
		map[string]string{"oldPath": "RENAMED.md", "newPath": "AGAIN.md"}, http.StatusBadRequest).
		Body.Close()
}
