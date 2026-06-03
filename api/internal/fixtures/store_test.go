package fixtures_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/fixtures"
)

func TestStore_GetWorkspace_Found(t *testing.T) {
	s := fixtures.NewStore()
	ws := fixtures.WorkspacePayload{ID: "ws1", RepoID: "crowbar", Branch: "main"}
	s.AddWorkspace(ws)

	got, ok := s.GetWorkspace("ws1")
	if !ok {
		t.Fatal("expected workspace to be found")
	}
	if got.ID != "ws1" {
		t.Fatalf("expected id ws1, got %s", got.ID)
	}
}

func TestStore_GetWorkspace_Missing(t *testing.T) {
	s := fixtures.NewStore()
	_, ok := s.GetWorkspace("missing")
	if ok {
		t.Fatal("expected workspace not found")
	}
}

func TestStore_AddProject(t *testing.T) {
	s := fixtures.NewStore()
	s.AddProject(fixtures.Project{ID: "p1", Name: "Crowbar"})
	projects := s.ListProjects()
	if len(projects) != 1 {
		t.Fatalf("expected 1 project, got %d", len(projects))
	}
}
