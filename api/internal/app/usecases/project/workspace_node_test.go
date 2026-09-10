package project_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/defaultbranch"
	"github.com/char2cs/crowbar/api/internal/app/usecases/mocks"
	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

// TestImport_EveryCreatedWorkspaceHasItsOwnNodeRow is 2026-09-08
// sidebar-placement-unification Task 7's core invariant test: a workspace
// created via project import — the project's own home, a repo's adopted home,
// a protected branch's managed worktree, and a protected branch's placeholder
// (no live holder is free here, so provisionProtectedBranchWorktree takes the
// managed-worktree branch and createPlaceholderWorkspace takes the
// held-by-home branch) — gets a Node{Kind:workspace} row minted immediately,
// with no separate backfill or lazy-mint step. It reuses
// TestImport_CreatesProjectReposAndAdoptsWorktrees' own fixture shape (four
// workspaces from one Import call) so every project-package workspace-creation
// path is proven in one pass.
func TestImport_EveryCreatedWorkspaceHasItsOwnNodeRow(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()
	nodes := mocks.NewNodePlacements()
	uc := newImportUsecase(project.ImportDeps{
		Projects:   projects,
		Repos:      repos,
		Workspaces: ws,
		Git:        git,
		Provider:   prov,
		Nodes:      nodes,
		Discover: func(_ string, _ int) ([]string, error) {
			return []string{"/repoA"}, nil
		},
		RefRunner: func(_ string) defaultbranch.RefRunner {
			return func(args ...string) (string, bool) {
				if len(args) > 0 && args[0] == "symbolic-ref" && len(args) == 1 {
					return "refs/remotes/origin/main", true
				}
				return "", false
			}
		},
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		CrowbarHome: func() (string, error) { return "/crowbar-home", nil },
		Stat:        statExists,
	})

	git.Worktrees = []gitengine.WorktreeEntry{
		{Path: "/repoA", Branch: "main", Head: "h1"},
		{Path: "/repoA/wt-feature", Branch: "feature", Head: "h2"},
	}
	git.MergeBaseSha = "forksha"
	git.RemoteBranches = map[string]bool{"develop": true}
	prov.Protected = []string{"main", "develop"}

	_, err := uc.Import(context.Background(), "My Project", "/root")
	require.NoError(t, err)

	require.Len(t, ws.Created, 4, "project home, repo home, a placeholder and a managed worktree")
	for _, w := range ws.Created {
		n, err := nodes.GetNode(context.Background(), w.ID)
		require.NoErrorf(t, err, "workspace %s (kind=%v) must have a Node row the instant it is created", w.ID, w.Kind)
		assert.Equal(t, domain.NodeKindWorkspace, n.Kind)
		assert.Equal(t, w.ID, n.ID)
	}
}

// TestImport_WorkspaceNodeMintFails_RollsBackTheWorkspaceAndChat proves the
// rollback half of the invariant: when the Node surface fails to mint a
// workspace's own row, the workspace row and the chat minted to own it are
// BOTH taken back out — the same orphan-free guarantee AttachOwningWorkspace
// failing already gets, now extended to the Node mint that runs just before
// it.
func TestImport_WorkspaceNodeMintFails_RollsBackTheWorkspaceAndChat(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	chats := newFakeOwningChats()
	nodes := mocks.NewNodePlacements()
	nodes.CreateErr = errors.New("node create boom")
	uc := project.NewImport(project.ImportDeps{
		Projects:   projects,
		Repos:      mocks.NewRepositoryStore(),
		Workspaces: ws,
		Git:        mocks.NewGitEngine(),
		Provider:   mocks.NewProviderEngine(),
		Discover:   func(string, int) ([]string, error) { return nil, nil },
		RefRunner:  noRefRunner,
		Nodes:      nodes,
		Now:        func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:       statExists,
	})
	uc.SetOwningChats(chats)

	_, err := uc.Import(context.Background(), "P", "/root")

	require.Error(t, err)
	require.Len(t, ws.Created, 1, "the row was written before the node mint was attempted")
	assert.Equal(t, []string{ws.Created[0].ID}, ws.Deleted,
		"the workspace whose own node row could not be minted must be taken back out")
	assert.Len(t, chats.discards(), 1, "the chat minted for it goes too")
	assert.Empty(t, projects.Saved, "the project row rolls back with the home that failed")
}
