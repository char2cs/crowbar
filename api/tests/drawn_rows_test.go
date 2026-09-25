//go:build integration

package tests

import (
	"sort"
	"strings"
	"testing"
)

// drawnRow is one repo-root row with everything the sidebar's own comparator
// (row-order.ts compareSidebarRows: order, kind rank, createdAt, id) reads.
type drawnRow struct {
	id        string
	kind      string
	order     int
	createdAt string
}

func rankOfKind(kind string) int {
	switch kind {
	case "folder":
		return 0
	case "branch":
		return 1
	case "chat":
		return 2
	}
	return 3
}

// drawnRepoRoot reads a repo's root level and sorts it exactly as the sidebar
// draws it (rows-from-repo.ts: a locked branch row carries its WORKSPACE's
// createdAt, a chat row its own).
func drawnRepoRoot(t *testing.T, h *harness, imported importedRepo, defaultWS string) []drawnRow {
	t.Helper()
	var rows []drawnRow
	var chats []struct {
		ID          string `json:"id"`
		WorkspaceID string `json:"workspaceId"`
		ParentID    string `json:"parentId"`
		Order       int    `json:"order"`
		CreatedAt   string `json:"createdAt"`
		Worktree    *struct {
			OwningChatID string `json:"owningChatId"`
		} `json:"worktree"`
	}
	h.get(repoBase(imported)+"/chats", &chats)
	for _, c := range chats {
		if c.ParentID != "" || c.WorkspaceID != defaultWS {
			continue
		}
		if c.Worktree != nil && c.Worktree.OwningChatID == c.ID {
			continue
		}
		rows = append(rows, drawnRow{id: c.ID, kind: "chat", order: c.Order, createdAt: c.CreatedAt})
	}
	var workspaces []struct {
		ID        string `json:"id"`
		IsDefault bool   `json:"isDefault"`
		FolderID  string `json:"folderId"`
		Order     int    `json:"order"`
		CreatedAt string `json:"createdAt"`
	}
	h.get(repoBase(imported)+"/workspaces", &workspaces)
	for _, w := range workspaces {
		if w.IsDefault || w.FolderID != "" {
			continue
		}
		rows = append(rows, drawnRow{id: w.ID, kind: "branch", order: w.Order, createdAt: w.CreatedAt})
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

func drawnIDs(rows []drawnRow) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, r.id)
	}
	return out
}
