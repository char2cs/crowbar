package main

import "testing"

// Import leaves two rows reporting the default branch: the isDefault "home"
// row for the unmanaged repo folder, and the locked worktree Crowbar
// actually owns. Only the locked one is a legal fork parent.
func TestPickBaseChatSkipsTheHomeRow(t *testing.T) {
	list := []chatDTO{
		{ID: "home", Worktree: &chatWorktreeDTO{Branch: "main", IsDefault: true, LocalPath: "/repo"}},
		{ID: "base", Worktree: &chatWorktreeDTO{Branch: "main", Status: "locked", LocalPath: "/wt/main"}},
	}

	got, ok := pickBaseChat("main")(list)
	if !ok {
		t.Fatal("expected the locked main chat to be found")
	}
	if got.ID != "base" {
		t.Fatalf("picked %q, want the locked base chat", got.ID)
	}
}

func TestPickBaseChatIgnoresDeletedRows(t *testing.T) {
	list := []chatDTO{{ID: "gone", Worktree: &chatWorktreeDTO{Branch: "main", Status: statusDeleted}}}

	if _, ok := pickBaseChat("main")(list); ok {
		t.Fatal("a deleted chat must never be chosen as the fork parent")
	}
}

func TestPickBaseChatIgnoresChatsWithNoWorktree(t *testing.T) {
	list := []chatDTO{{ID: "bubble"}}

	if _, ok := pickBaseChat("main")(list); ok {
		t.Fatal("a chat that owns no worktree must never be chosen as the fork parent")
	}
}

func TestPickBaseChatHonoursANonMainDefaultBranch(t *testing.T) {
	list := []chatDTO{
		{ID: "base", Worktree: &chatWorktreeDTO{Branch: "trunk", Status: "locked"}},
		{ID: "other", Worktree: &chatWorktreeDTO{Branch: "main", Status: "new"}},
	}

	got, _ := pickBaseChat("trunk")(list)
	if got.ID != "base" {
		t.Fatalf("picked %q, want the trunk chat", got.ID)
	}
}

// The seed commits into the feature chat's worktree, so a row whose worktree
// is not on disk yet is not usable and the poll must keep waiting.
func TestPickFeatureChatWaitsForTheWorktreeOnDisk(t *testing.T) {
	list := []chatDTO{{ID: "ws", Worktree: &chatWorktreeDTO{Branch: seedFeatureBranch, LocalPath: ""}}}

	if _, ok := pickFeatureChat(list); ok {
		t.Fatal("a chat without a worktree path must not be accepted")
	}
}

func TestPickFeatureChatMatchesTheSeedBranch(t *testing.T) {
	list := []chatDTO{
		{ID: "other", Worktree: &chatWorktreeDTO{Branch: "feature/unrelated", LocalPath: "/wt/other"}},
		{ID: "ws", Worktree: &chatWorktreeDTO{Branch: seedFeatureBranch, LocalPath: "/wt/seed"}},
	}

	got, ok := pickFeatureChat(list)
	if !ok || got.ID != "ws" {
		t.Fatalf("picked %+v, want the %s chat", got, seedFeatureBranch)
	}
}

func TestPickFeatureChatIgnoresDeletedRows(t *testing.T) {
	list := []chatDTO{{ID: "gone", Worktree: &chatWorktreeDTO{Branch: seedFeatureBranch, Status: statusDeleted, LocalPath: "/wt/gone"}}}

	if _, ok := pickFeatureChat(list); ok {
		t.Fatal("a deleted feature chat must never be reused")
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

func TestChatsPathIsRepoScopedWithNoWorkspace(t *testing.T) {
	if got := chatsPath("p", "r"); got != "/v0/projects/p/repos/r/chats" {
		t.Fatalf("path = %q", got)
	}
}

func TestChatDetailPathNamesTheChatUnderItsRepo(t *testing.T) {
	if got := chatDetailPath("p", "r", "c"); got != "/v0/projects/p/repos/r/chats/c" {
		t.Fatalf("path = %q", got)
	}
}

func TestFlatChatPathCarriesNoProjectOrRepo(t *testing.T) {
	if got := flatChatPath("c", "/identity"); got != "/v0/chats/c/identity" {
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
