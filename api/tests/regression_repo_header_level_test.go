//go:build integration

package tests

import (
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func repoHeaderLevel(t *testing.T, h *harness, imported importedRepo, headerChatID string) []topRow {
	t.Helper()
	base := repoBase(imported)
	var rows []topRow
	var chats []struct {
		ID       string `json:"id"`
		ParentID string `json:"parentId"`
		Order    int    `json:"order"`
	}
	h.get(base+"/chats", &chats)
	for _, c := range chats {
		if c.ID == headerChatID || c.ParentID != headerChatID {
			continue
		}
		rows = append(rows, topRow{id: c.ID, kind: "chat", order: c.Order})
	}
	for _, f := range listChatFolders(t, h, base) {
		if f.ParentID != "" {
			continue
		}
		rows = append(rows, topRow{id: f.ID, kind: "folder", order: f.Order})
	}
	var workspaces []struct {
		ID        string `json:"id"`
		IsDefault bool   `json:"isDefault"`
		Status    string `json:"status"`
		FolderID  string `json:"folderId"`
		Order     int    `json:"order"`
	}
	h.get(base+"/workspaces", &workspaces)
	for _, w := range workspaces {
		if w.IsDefault || w.Status != "locked" || w.FolderID != "" {
			continue
		}
		rows = append(rows, topRow{id: w.ID, kind: "branch", order: w.Order})
	}
	rank := map[string]int{"folder": 0, "branch": 1, "chat": 2}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].order != rows[j].order {
			return rows[i].order < rows[j].order
		}
		return rank[rows[i].kind] < rank[rows[j].kind]
	})
	return rows
}

func TestRegression_ThreadOnRepoHeaderCannotBeOrderedBeforeALockedBranch(t *testing.T) {
	h := newHarness(t)
	writePromoteStubProviderDescriptor(t, h)
	imported := importProject(t, h)
	base := repoBase(imported)

	var workspaces []workspaceDTO
	h.get(base+"/workspaces", &workspaces)
	var headerChatID, defaultWsID string
	for _, w := range workspaces {
		if w.IsDefault {
			headerChatID, defaultWsID = w.OwningChatID, w.ID
		}
	}
	require.NotEmpty(t, headerChatID, "the default checkout must have an owning chat")

	t1 := createChatWithProvider(t, h, base, "promotestub", defaultWsID, headerChatID)
	t2 := createChatWithProvider(t, h, base, "promotestub", defaultWsID, headerChatID)
	h.QuiesceReactors()
	h.Quiesce()

	before := repoHeaderLevel(t, h, imported, headerChatID)
	t.Logf("before: %+v", before)
	require.Len(t, before, 3, "locked main + two header threads")
	h.patch(base+"/chats/"+t2+"/placement", map[string]any{"parentId": headerChatID, "order": 0}, nil)
	h.Quiesce()

	after := repoHeaderLevel(t, h, imported, headerChatID)
	t.Logf("after t2->0: %+v", after)
	assertDense(t, after)
	assert.Equal(t, t2, after[0].id, "t2 must land above the locked branch")
	_ = t1
}
