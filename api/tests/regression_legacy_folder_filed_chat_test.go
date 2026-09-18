//go:build integration

package tests

import (
	"context"
	"net/http"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// drawnTopLevel reads the project top level as rows-from-home.ts/rows-from-repo.ts
// draw it: a chat whose parentId names no row it knows falls back to its
// ground (workspace-tree-utils.ts buildSidebarTree: `filed = nodes.has(parentId)
// ? parentId : undefined`), so it is drawn at the top level. Sorted with the
// sidebar's own comparator (order, kind rank, createdAt, id).
func drawnTopLevel(t *testing.T, h *harness, projectID, homeOwnerID string, known map[string]bool) []drawnRow {
	t.Helper()
	var rows []drawnRow
	var chats []struct {
		ID        string `json:"id"`
		ParentID  string `json:"parentId"`
		Order     int    `json:"order"`
		CreatedAt string `json:"createdAt"`
	}
	h.get("/v0/projects/"+projectID+"/home/chats", &chats)
	for _, c := range chats {
		if c.ID == homeOwnerID || (c.ParentID != "" && known[c.ParentID]) {
			continue
		}
		rows = append(rows, drawnRow{id: c.ID, kind: "chat", order: c.Order, createdAt: c.CreatedAt})
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
		rows = append(rows, drawnRow{id: r.ID, kind: "repo", order: r.Order})
	}
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.order != b.order {
			return a.order < b.order
		}
		if rankOfKind(a.kind) != rankOfKind(b.kind) {
			return rankOfKind(a.kind) < rankOfKind(b.kind)
		}
		if a.createdAt != b.createdAt {
			return a.createdAt < b.createdAt
		}
		return strings.Compare(a.id, b.id) < 0
	})
	return rows
}

// legacyFolderFiledChat turns a home chat into the shape every chat filed
// into a pre-#179 Chats-panel folder (the retired agent_chat_folders table,
// never migrated) has today: no Node row, and a frozen Chat.ParentID naming a
// folder id nothing answers to any more.
func legacyFolderFiledChat(t *testing.T, h *harness, chatID string) {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, h.app.Repositories.Node.Forget(ctx, chatID))
	_, err := h.app.Repositories.AgentChat.SetPlacement(ctx, chatID, "legacy-agent-chat-folder", 0)
	require.NoError(t, err)
	h.Quiesce()
}

// A chat filed into a folder that no longer exists is drawn at the project
// top level, but neither top-level writer counts it there: the repo path
// (homeLevel/withNodelessRows) skips a chat whose ParentID != "", and the
// chat path files it under its ghost container. Every drop around it lands
// one slot off.
func TestRegression_LegacyFolderFiledChatIsDrawnAtTopLevelButNeverCounted(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	p := imported.projectID
	var home struct {
		OwningChatID string `json:"owningChatId"`
	}
	h.get("/v0/projects/"+p+"/home", &home)

	ghostFiled := createHomeChatStub(t, h, p)
	legacyFolderFiledChat(t, h, ghostFiled)
	c2 := createHomeChatStub(t, h, p)

	known := map[string]bool{ghostFiled: true, c2: true, imported.repoID: true}
	before := drawnTopLevel(t, h, p, home.OwningChatID, known)
	t.Logf("drawn before: %+v", before)
	require.Equal(t, []string{ghostFiled, imported.repoID, c2}, drawnIDs(before))

	// Drag c2 above the repo: rest = [ghostFiled, repo], drawn index 1.
	h.patch("/v0/projects/"+p+"/home/chats/"+c2+"/placement", map[string]any{"parentId": "", "order": 1}, nil)
	h.Quiesce()
	after := drawnTopLevel(t, h, p, home.OwningChatID, known)
	t.Logf("drawn after c2->1: %+v", after)
	assert.Equal(t, []string{ghostFiled, c2, imported.repoID}, drawnIDs(after),
		"c2 must land between the legacy chat and the repo")

	// And the repo to the very top: rest = [ghostFiled, c2], drawn index 0.
	h.raw(http.MethodPatch, "/v0/projects/"+p+"/repos/"+imported.repoID, map[string]any{"order": 0}, http.StatusNoContent).Body.Close()
	h.Quiesce()
	after2 := drawnTopLevel(t, h, p, home.OwningChatID, known)
	t.Logf("drawn after repo->0: %+v", after2)
	assert.Equal(t, imported.repoID, after2[0].id, "the repo must be drawn first")
}
