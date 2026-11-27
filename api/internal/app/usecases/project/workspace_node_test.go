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
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	gitengine "github.com/char2cs/crowbar/api/internal/engine/git"
)

func TestImport_EveryCreatedWorkspaceHasItsOwnNodeRow(t *testing.T) {
	projects := mocks.NewProjectStore()
	repos := mocks.NewRepositoryStore()
	ws := mocks.NewWorkspaceRepo()
	git := mocks.NewGitEngine()
	prov := mocks.NewProviderEngine()
	nodes := mocks.NewNodePlacements()
	uc := newImportUsecase(t, project.ImportDeps{
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
		assert.Equal(t, tree.KindWorkspace, n.Kind)
		assert.Equal(t, w.ID, n.ID)
	}
}

func TestImport_WorkspaceNodeMintFails_RollsBackTheWorkspaceAndChat(t *testing.T) {
	projects := mocks.NewProjectStore()
	ws := mocks.NewWorkspaceRepo()
	chats := newFakeOwningChats()
	nodes := mocks.NewNodePlacements()
	nodes.CreateErr = errors.New("node create boom")
	uc := project.NewImport(project.ImportDeps{
		CrowbarHome: tempCrowbarHome(t),
		Projects:    projects,
		Repos:       mocks.NewRepositoryStore(),
		Workspaces:  ws,
		Git:         mocks.NewGitEngine(),
		Provider:    mocks.NewProviderEngine(),
		Discover:    func(string, int) ([]string, error) { return nil, nil },
		RefRunner:   noRefRunner,
		Nodes:       nodes,
		Now:         func() time.Time { return time.Unix(1000, 0).UTC() },
		Stat:        statExists,
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
