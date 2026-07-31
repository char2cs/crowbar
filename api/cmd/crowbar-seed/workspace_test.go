package main

import "testing"

// Import leaves two rows reporting the default branch: the isDefault "home" row
// for the unmanaged repo folder, and the locked worktree Crowbar actually owns.
// Only the locked one is a legal fork parent.
func TestPickBaseWorkspaceSkipsTheHomeRow(t *testing.T) {
	list := []workspaceDTO{
		{ID: "home", RepoID: "r1", Branch: "main", IsDefault: true, LocalPath: "/repo"},
		{ID: "base", RepoID: "r1", Branch: "main", Status: "locked", LocalPath: "/wt/main"},
	}

	got, ok := pickBaseWorkspace("r1", "main")(list)
	if !ok {
		t.Fatal("expected the locked main workspace to be found")
	}
	if got.ID != "base" {
		t.Fatalf("picked %q, want the locked base workspace", got.ID)
	}
}

func TestPickBaseWorkspaceIgnoresDeletedRows(t *testing.T) {
	list := []workspaceDTO{{ID: "gone", RepoID: "r1", Branch: "main", Status: statusDeleted}}

	if _, ok := pickBaseWorkspace("r1", "main")(list); ok {
		t.Fatal("a deleted workspace must never be chosen as the fork parent")
	}
}

func TestPickBaseWorkspaceHonoursANonMainDefaultBranch(t *testing.T) {
	list := []workspaceDTO{
		{ID: "base", RepoID: "r1", Branch: "trunk", Status: "locked"},
		{ID: "other", RepoID: "r1", Branch: "main", Status: "new"},
	}

	got, _ := pickBaseWorkspace("r1", "trunk")(list)
	if got.ID != "base" {
		t.Fatalf("picked %q, want the trunk workspace", got.ID)
	}
}

// The seed commits into the workspace's worktree, so a row whose worktree is
// not on disk yet is not usable and the poll must keep waiting.
func TestPickFeatureWorkspaceWaitsForTheWorktreeOnDisk(t *testing.T) {
	list := []workspaceDTO{{ID: "ws", RepoID: "r1", Branch: seedFeatureBranch, LocalPath: ""}}

	if _, ok := pickFeatureWorkspace("r1")(list); ok {
		t.Fatal("a workspace without a worktree path must not be accepted")
	}
}

func TestPickFeatureWorkspaceMatchesTheSeedBranch(t *testing.T) {
	list := []workspaceDTO{
		{ID: "other", RepoID: "r1", Branch: "feature/unrelated", LocalPath: "/wt/other"},
		{ID: "ws", RepoID: "r1", Branch: seedFeatureBranch, LocalPath: "/wt/seed"},
	}

	got, ok := pickFeatureWorkspace("r1")(list)
	if !ok || got.ID != "ws" {
		t.Fatalf("picked %+v, want the %s workspace", got, seedFeatureBranch)
	}
}

func TestPickProjectMatchesOnlyTheSeedProject(t *testing.T) {
	list := []projectDTO{{ID: "a", Name: "Something Else"}, {ID: "b", Name: seedProjectName}}

	got, ok := pickProject(list)
	if !ok || got.ID != "b" {
		t.Fatalf("picked %+v, want the seed project", got)
	}
}

func TestPickProjectIgnoresUnrelatedProjects(t *testing.T) {
	if _, ok := pickProject([]projectDTO{{ID: "a", Name: "Task18"}}); ok {
		t.Fatal("the seed must never adopt an unrelated project")
	}
}

// GET /v0/projects/:projectId/repos answers with every repo the daemon knows,
// so a same-named repo under another project must not be adopted.
func TestPickRepoRejectsAnotherProjectsRepo(t *testing.T) {
	list := []repoDTO{{ID: "a", ProjectID: "other", Name: seedRepoName}}

	if _, ok := pickRepo("p1")(list); ok {
		t.Fatal("the seed must not adopt a same-named repo from another project")
	}
}

func TestPickBaseWorkspaceRejectsAnotherReposRows(t *testing.T) {
	list := []workspaceDTO{{ID: "base", RepoID: "other", Branch: "main", Status: "locked"}}

	if _, ok := pickBaseWorkspace("r1", "main")(list); ok {
		t.Fatal("the fork parent must come from the seed repo")
	}
}

func TestPickRepoMatchesOnlyTheSeedRepo(t *testing.T) {
	list := []repoDTO{
		{ID: "a", ProjectID: "p1", Name: "demo"},
		{ID: "b", ProjectID: "p1", Name: seedRepoName},
	}

	got, ok := pickRepo("p1")(list)
	if !ok || got.ID != "b" {
		t.Fatalf("picked %+v, want the seed repo", got)
	}
}

func TestScopePathNestsUnderTheWorkspace(t *testing.T) {
	sc := scope{projectID: "p", repoID: "r", workspaceID: "w"}

	if got := sc.path("/threads"); got != "/v0/projects/p/repos/r/workspaces/w/threads" {
		t.Fatalf("path = %q", got)
	}
}

func TestThreadExistsMatchesOnFileAndLine(t *testing.T) {
	existing := []threadDTO{{FilePath: pricingPath, Line: 21}}

	if !threadExists(existing, pricingPath, 21) {
		t.Fatal("an already-seeded thread must be recognised")
	}
	if threadExists(existing, pricingPath, 42) {
		t.Fatal("a different line is a different thread")
	}
	if threadExists(existing, "src/inventory.ts", 21) {
		t.Fatal("a different file is a different thread")
	}
}
