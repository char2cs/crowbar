//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// This file pins repo/project scoping end to end, driving the real HTTP +
// WebSocket surface so a refusal that only ever existed in the frontend would
// fail here.
//
// It used to also pin the sidebar's now-retired folder feature
// (domain.Folder, the /folders REST resource, and the dense Order
// Workspace.FolderID/Order shared with it) — all deleted onto the unified
// chat/folder tree (usecases/chat/internal/tree), which the Chats-panel's own
// regression suite (regression_agent_chat_folders_test.go) already covers.
// These two tests never touched that feature and are kept.

// repoBase returns the hierarchical repo route prefix for an imported repo.
func repoBase(
	imported importedRepo,
) string {
	return "/v0/projects/" + imported.projectID + "/repos/" + imported.repoID
}

// A repo that changed projects while its workspaces did not would KEEP them and
// stop showing them: every hierarchical route and the WS namespace are keyed on
// the workspace's own projectId, so they would 404 under the new path and be
// unreachable under the old.
// The repo list is scoped by the project in its PATH, not by a query parameter.
//
// Reading the wrong one is invisible with a single project — an unfiltered list
// and a correctly filtered one are the same list — and catastrophic with two:
// every project renders every repo in the install, so the sidebar shows the same
// repo under each project and no tree beneath it belongs where it is drawn.
//
// This needs a SECOND project to fail, which is exactly why nothing caught it.
func TestRegression_RepoListIsScopedToTheProjectInThePath(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	otherPath := gitRepoWithCommit(t)
	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "second", "path": otherPath}, http.StatusAccepted)
	_ = resp.Body.Close()
	other := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["path"] == otherPath
	})
	otherID, _ := other["id"].(string)
	require.NotEmpty(t, otherID)
	require.NotEqual(t, imported.projectID, otherID)

	h.Quiesce()

	var mine []repoDTO
	h.get("/v0/projects/"+imported.projectID+"/repos", &mine)
	require.NotEmpty(t, mine, "precondition: the first project has a repo")
	for _, r := range mine {
		assert.Equal(t, imported.projectID, r.ProjectID,
			"repo %s belongs to %s but was listed under %s", r.ID, r.ProjectID, imported.projectID)
	}

	var theirs []repoDTO
	h.get("/v0/projects/"+otherID+"/repos", &theirs)
	for _, r := range theirs {
		assert.Equal(t, otherID, r.ProjectID,
			"repo %s belongs to %s but was listed under %s", r.ID, r.ProjectID, otherID)
	}

	// The decisive assertion: the first project's repo must not appear under the
	// second. Without the path-parameter fix both lists are the whole install.
	for _, r := range theirs {
		assert.NotEqual(t, imported.repoID, r.ID,
			"the other project listed a repo that is not its own")
	}
}

func TestRegression_RepoMovedBetweenProjectsKeepsItsWorkspaces(t *testing.T) {
	h := newHarness(t)
	imported := importProject(t, h)

	// A second project to move the repo into.
	otherPath := gitRepoWithCommit(t)
	projectsWS := h.dial("/v0/projects")
	resp := h.raw(http.MethodPost, "/v0/projects",
		map[string]string{"name": "other", "path": otherPath}, http.StatusAccepted)
	_ = resp.Body.Close()
	other := readUntil(t, projectsWS, func(m map[string]any) bool {
		return m["path"] == otherPath
	})
	otherID, _ := other["id"].(string)
	require.NotEmpty(t, otherID)

	// The read model is eventually consistent, and the move reads it to find the
	// workspaces it has to carry. Quiesce is the deterministic barrier.
	h.Quiesce()
	before := listWorkspaces(t, h, imported.projectID, imported.repoID)
	require.NotEmpty(t, before, "precondition: the repo has workspaces to carry")

	resp = h.raw(http.MethodPatch, repoBase(imported),
		map[string]string{"projectId": otherID}, http.StatusNoContent)
	_ = resp.Body.Close()
	h.Quiesce()

	moved := listWorkspaces(t, h, otherID, imported.repoID)
	require.Len(t, moved, len(before), "every workspace follows the repo")
	for _, ws := range moved {
		assert.Equal(t, otherID, ws.ProjectID)
	}

	var repos []repoDTO
	h.get("/v0/projects/"+otherID+"/repos", &repos)
	var found bool
	for _, repo := range repos {
		if repo.ID == imported.repoID {
			found = true
		}
	}
	assert.True(t, found, "the repo itself is listed under its new project")

	// And it is gone from the old one — the repo-scope guard 404s the stale path
	// rather than serving a repo from a project that no longer owns it.
	oldResp := h.raw(http.MethodGet, repoBase(imported), nil, http.StatusNotFound)
	_ = oldResp.Body.Close()
}
