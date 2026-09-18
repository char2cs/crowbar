//go:build integration

package tests

import (
	"context"
	"net/http"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type topRow struct {
	id    string
	kind  string
	order int
}

// topLevel reads the flat project top level exactly as the sidebar draws it:
// home chats/folders at root (parentId "") plus every repo header, sorted by
// order — a repo's order comes off GET .../repos, a chat's off GET home/chats.
func topLevel(t *testing.T, h *harness, projectID, homeOwnerID string) []topRow {
	t.Helper()
	var rows []topRow
	var chats []struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		Order    int    `json:"order"`
		Type     string `json:"type"`
	}
	h.get("/v0/projects/"+projectID+"/home/chats", &chats)
	for _, c := range chats {
		if c.ParentID != "" || c.ID == homeOwnerID {
			continue
		}
		rows = append(rows, topRow{id: c.ID, kind: "chat", order: c.Order})
	}
	var repos []struct {
		ID       string `json:"id"`
		FolderID string `json:"folderId"`
		Order    int    `json:"order"`
	}
	h.get("/v0/projects/"+projectID+"/repos", &repos)
	for _, r := range repos {
		if r.FolderID != "" {
			continue
		}
		rows = append(rows, topRow{id: r.ID, kind: "repo", order: r.Order})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].order < rows[j].order })
	return rows
}

func createHomeChatStub(t *testing.T, h *harness, projectID string) string {
	t.Helper()
	var created struct {
		ID string `json:"id"`
	}
	h.post("/v0/projects/"+projectID+"/home/chats", map[string]string{"provider": "promotestub"},
		http.StatusCreated, &created)
	require.NotEmpty(t, created.ID)
	h.QuiesceReactors()
	h.Quiesce()
	return created.ID
}

func idsOf(rows []topRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.id)
	}
	return out
}

func assertDense(t *testing.T, rows []topRow) {
	t.Helper()
	for i, r := range rows {
		assert.Equalf(t, i, r.order, "row %d (%s %s) must carry dense order", i, r.kind, r.id)
	}
}

// K3: a project's top level is ONE interleaved list of repo headers and home
// chats. Moving a repo between two chats, and a chat past a repo, must both
// change what the two reads render.
func TestRegression_TopLevelReorderRepoAmongHomeChats(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID

	// the project home's own owning chat is not a row
	var home struct {
		OwningChatID string `json:"owningChatId"`
		ID           string `json:"id"`
	}
	h.get("/v0/projects/"+p+"/home", &home)

	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)

	before := topLevel(t, h, p, home.OwningChatID)
	t.Logf("before: %+v", before)
	require.Len(t, before, 3)

	// Drag the repo to slot 1 (between c1 and c2) — the exact write
	// drop-actions.ts's 'repoHome' call makes.
	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+imported.repoID, map[string]any{"order": 1}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	after := topLevel(t, h, p, home.OwningChatID)
	t.Logf("after repo->1: %+v", after)
	assertDense(t, after)
	assert.Equal(t, imported.repoID, after[1].id, "repo must sit at slot 1")

	// Now drag c1 (currently slot 0) to the tail (slot 2) — the 'chat' call.
	var placed struct {
		Chat struct {
			ID    string `json:"id"`
			Order int    `json:"order"`
		} `json:"chat"`
	}
	h.patch("/v0/projects/"+p+"/home/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 2}, &placed)
	h.Quiesce()
	after2 := topLevel(t, h, p, home.OwningChatID)
	t.Logf("after c1->2: %+v", after2)
	assertDense(t, after2)
	assert.Equal(t, []string{imported.repoID, c2, c1}, idsOf(after2))

	// And the repo back to the tail.
	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+imported.repoID, map[string]any{"order": 2}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	after3 := topLevel(t, h, p, home.OwningChatID)
	t.Logf("after repo->2: %+v", after3)
	assertDense(t, after3)
	assert.Equal(t, []string{c2, c1, imported.repoID}, idsOf(after3))
}

// K3 tie-break: with EVERY top-level row at order 0 (the dev data's state), a
// single reorder write must still yield a dense, distinct order.
func TestRegression_TopLevelReorderFromAllZeroTies(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	var home struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+p+"/home", &home)
	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)
	rows := topLevel(t, h, p, home.OwningChatID)
	t.Logf("fresh: %+v", rows)
	// A fresh project should already be dense; if not, that is the tie bug.
	assertDense(t, rows)

	h.patch("/v0/projects/"+p+"/home/chats/"+c2+"/placement", map[string]any{"parentId": "", "order": 0}, nil)
	h.Quiesce()
	rows = topLevel(t, h, p, home.OwningChatID)
	t.Logf("c2->0: %+v", rows)
	assertDense(t, rows)
	assert.Equal(t, c2, rows[0].id)
	_ = c1
}

// Legacy data: every top-level row tied at 0. The sidebar draws a tied level
// as folders, then chats, then repo headers (its arrival order), and both
// backend writers must break the same tie the same way, or the first drag
// lands somewhere other than the drop line and untouched rows jump.
func TestRegression_TopLevelFirstDropLandsWhereIndicated(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	var home struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+p+"/home", &home)
	c1 := createHomeChatStub(t, h, p)
	c2 := createHomeChatStub(t, h, p)
	ctx := context.Background()
	for _, id := range []string{imported.repoID, c1, c2} {
		require.NoError(t, h.app.Repositories.Node.SetOrder(ctx, id, 0))
	}
	h.Quiesce()

	// Displayed [c1, c2, repo]. Drag the repo BEFORE c1 (index 0).
	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+imported.repoID, map[string]any{"order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	rows := topLevel(t, h, p, home.OwningChatID)
	t.Logf("repo->0: %+v", rows)
	assertDense(t, rows)
	assert.Equal(t, []string{imported.repoID, c1, c2}, idsOf(rows))

	// Tie everything again; displayed [c1, c2, repo]. Drag c1 AFTER the repo
	// (index 2) through the chat writer.
	for _, id := range []string{imported.repoID, c1, c2} {
		require.NoError(t, h.app.Repositories.Node.SetOrder(ctx, id, 0))
	}
	h.Quiesce()
	h.patch("/v0/projects/"+p+"/home/chats/"+c1+"/placement", map[string]any{"parentId": "", "order": 2}, nil)
	h.Quiesce()
	rows = topLevel(t, h, p, home.OwningChatID)
	t.Logf("c1->2: %+v", rows)
	assertDense(t, rows)
	assert.Equal(t, []string{c2, imported.repoID, c1}, idsOf(rows))
}
