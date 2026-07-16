//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// createProjectForHome creates a project via POST /v0/projects and returns its
// id once the ProjectDTO is broadcast. Unlike importProject it does NOT wait for
// repo discovery: POST /v0/projects provisions only the project record and its
// home workspace (repo setup is a separate flow), which is all the home suite
// needs. Using a real git dir gives the home workspace (rooted at the project
// path) a populated tree for the file routes.
func createProjectForHome(t *testing.T, h *harness) string {
	t.Helper()
	dir := gitRepoWithCommit(t)

	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "home-demo", "path": dir}, http.StatusAccepted)
	_ = resp.Body.Close()

	project := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["path"] == dir
	})
	id, _ := project["id"].(string)
	require.NotEmpty(t, id, "project create must broadcast a ProjectDTO with an id")
	return id
}

// TestRegression_HomeEndpointReachable proves that GET /v0/projects/:id/home
// returns 200 with a workspace DTO whose kind is "home". This guards the router
// wiring: if the home package is not mounted under projectScoped the endpoint
// 404s and the project home panel never loads.
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

// TestHome_ThreadsRoundTrip proves review threads are a home-workspace capability:
// open a thread under /v0/projects/:p/home/threads and read it back. The home
// workspace has no repo, so this exercises the empty-repoId thread path end to
// end (open → store → list), which would 404 before the home thread routes were
// mounted.
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

// TestHome_ThreadBroadcastsOverWebSocket proves the dual-served home thread list
// works as a WebSocket: a subscriber on /home/threads receives the DTO broadcast
// when a thread is opened. This validates WS scoping for the repoId-less home
// workspace (namespace "p//w" prefix-matches the event "p//w/<id>").
func TestHome_ThreadBroadcastsOverWebSocket(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)
	homeBase := "/v0/projects/" + projectID + "/home"

	conn := h.dial(homeBase + "/threads")
	h.post(homeBase+"/threads", map[string]any{
		"filePath": "README.md", "line": 1, "side": "right", "body": "ws home thread",
	}, http.StatusCreated, nil)

	frame := readUntil(t, conn, func(m map[string]any) bool {
		return m["body"] == "ws home thread"
	})
	assert.Equal(t, projectID, frame["projectId"],
		"home thread frame must carry the project scope")
}

// TestHome_FilesWatchWebSocketUpgrades proves the home file-change stream route
// exists and upgrades. This is the exact route the project-home view hit a 404
// on (/v0/projects/:p/home/files/ws); the dial helper fails on a non-101 upgrade,
// so a successful dial is the regression guard.
func TestHome_FilesWatchWebSocketUpgrades(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)

	_ = h.dial("/v0/projects/" + projectID + "/home/files/ws")
}

// TestRegression_HomeHasNoGitSurface locks the design decision that git is NOT a
// home capability: the home workspace is the project root, not a per-workspace
// git worktree. The route stays 404 by design; the frontend must skip git
// streams for the home workspace rather than expect this to resolve.
func TestRegression_HomeHasNoGitSurface(t *testing.T) {
	h := newHarness(t)
	projectID := createProjectForHome(t, h)

	resp := h.raw(http.MethodGet,
		"/v0/projects/"+projectID+"/home/git/status", nil, http.StatusNotFound)
	_ = resp.Body.Close()
}

// TestRegression_HomeFilesCopy proves POST /home/files/copy is mounted for the
// project-home workspace and performs a byte-faithful duplicate: create/rename/
// delete were mounted on the home route group (Task 28), but the copy verb was
// not, so Duplicate in the home explorer 404'd every time (Task 34).
func TestRegression_HomeFilesCopy(t *testing.T) {
	h := newHarness(t)
	homeBase := "/v0/projects/" + createProjectForHome(t, h) + "/home"

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
	h.get(homeBase+"/files/content?path=README.md", &original)
	h.get(homeBase+"/files/content?path=README+copy.md", &dup)
	assert.Equal(t, original.Content, dup.Content,
		"home copy must reproduce the source content byte-for-byte")
}

// TestRegression_HomeFilesCopyConflictAndMissingSource proves the home copy
// route maps usecase errors exactly like the workspace-scoped verb it mirrors:
// an existing destination is a 409, a missing source is a 404.
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

// TestRegression_HomeFilesRenameContract proves PATCH /home/files renames on the
// shared {path, newPath} shape the workspace handler and the FE already send. The
// handler bound {oldPath, newPath}, so every home-project explorer rename 400'd
// on the mismatch and failed silently in the UI (Task 28/29/31 built out this
// explorer). A rename with {path, newPath} moves the file; the retired
// {oldPath, newPath} shape now 400s.
func TestRegression_HomeFilesRenameContract(t *testing.T) {
	h := newHarness(t)
	homeBase := "/v0/projects/" + createProjectForHome(t, h) + "/home"

	var before struct {
		Content string `json:"content"`
	}
	h.get(homeBase+"/files/content?path=README.md", &before)

	var renamed struct {
		ID string `json:"id"`
	}
	h.patch(homeBase+"/files",
		map[string]string{"path": "README.md", "newPath": "RENAMED.md"}, &renamed)
	assert.Equal(t, "RENAMED.md", renamed.ID)

	// Content survives at the new path; the old path is gone.
	var after struct {
		Content string `json:"content"`
	}
	h.get(homeBase+"/files/content?path=RENAMED.md", &after)
	assert.Equal(t, before.Content, after.Content, "rename must preserve content at the new path")
	h.raw(http.MethodGet, homeBase+"/files/content?path=README.md", nil, http.StatusNotFound).Body.Close()

	// The retired {oldPath, newPath} shape leaves `path` unbound → 400.
	h.raw(http.MethodPatch, homeBase+"/files",
		map[string]string{"oldPath": "RENAMED.md", "newPath": "AGAIN.md"}, http.StatusBadRequest).
		Body.Close()
}
