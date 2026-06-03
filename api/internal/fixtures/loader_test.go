package fixtures_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/fixtures"
)

func TestLoad_ReturnsPopulatedStore(t *testing.T) {
	store, err := fixtures.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	if len(store.Flows) == 0 {
		t.Fatal("expected flows to be loaded")
	}
	if len(store.ListWorkspaces()) == 0 {
		t.Fatal("expected workspaces to be loaded")
	}
	if len(store.ListProjects()) == 0 {
		t.Fatal("expected projects to be loaded")
	}
	if len(store.GitLog) == 0 {
		t.Fatal("expected git log to be loaded")
	}
	if len(store.GitBranches) == 0 {
		t.Fatal("expected git branches to be loaded")
	}
	if store.FileTree.Type != "directory" {
		t.Fatal("expected file tree root to be a directory")
	}
}

func TestLoad_WorkspacesHaveFlowsPopulated(t *testing.T) {
	store, err := fixtures.Load()
	if err != nil {
		t.Fatalf("Load() error: %v", err)
	}
	workspaces := store.ListWorkspaces()
	for _, ws := range workspaces {
		if ws.Flow.Name == "" {
			t.Fatalf("workspace %s has empty flow name", ws.ID)
		}
		if len(ws.Flow.States) == 0 {
			t.Fatalf("workspace %s has no flow states", ws.ID)
		}
	}
}
