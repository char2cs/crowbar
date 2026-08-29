package usecases

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type fakeWorkspaceGetter struct {
	ws  domain.Workspace
	err error
}

func (f fakeWorkspaceGetter) Get(
	_ context.Context,
	_ string,
) (domain.Workspace, error) {
	if f.err != nil {
		return domain.Workspace{}, f.err
	}
	return f.ws, nil
}

type fakeRepoGetter struct {
	repo *domain.Repository
	err  error
}

func (f fakeRepoGetter) FindByKey(
	_ context.Context,
	_ string,
) (*domain.Repository, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.repo, nil
}

// TestAgentWorkspaceReader_WorktreeDir_Success proves WorktreeDir resolves the
// owning project/repo from the workspace repo and returns the git worktree
// directory stored on the workspace read model's WorktreePath, rooted at the
// injected crowbarHome (not metadata.GetHomePath, so it never diverges from a
// hermetic test's overridden home).
func TestAgentWorkspaceReader_WorktreeDir_Success(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "w1",
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: "/home/crowbar/projects/p1/r1/workspaces/w1/worktree",
		}},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	home, projectID, repoID, worktree, err := r.WorktreeDir(context.Background(), "w1")
	require.NoError(t, err)
	assert.Equal(t, "/home/crowbar", home)
	assert.Equal(t, "p1", projectID)
	assert.Equal(t, "r1", repoID)
	assert.Equal(t, "/home/crowbar/projects/p1/r1/workspaces/w1/worktree", worktree)
}

// TestAgentWorkspaceReader_WorktreeDir_CrowbarHomeError proves a crowbarHome
// resolver failure short-circuits before the workspace is even looked up.
func TestAgentWorkspaceReader_WorktreeDir_CrowbarHomeError(t *testing.T) {
	wantErr := errors.New("no home")
	r := &agentWorkspaceReader{
		workspaces:  fakeWorkspaceGetter{err: errors.New("must not be reached")},
		crowbarHome: func() (string, error) { return "", wantErr },
	}

	_, _, _, _, err := r.WorktreeDir(context.Background(), "w1")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

// TestAgentWorkspaceReader_WorktreeDir_WorkspaceGetError proves an unknown
// workspace id surfaces the repo's error wrapped, not swallowed.
func TestAgentWorkspaceReader_WorktreeDir_WorkspaceGetError(t *testing.T) {
	wantErr := errors.New("workspace not found")
	r := &agentWorkspaceReader{
		workspaces:  fakeWorkspaceGetter{err: wantErr},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	_, _, _, _, err := r.WorktreeDir(context.Background(), "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

// TestAgentWorkspaceReader_AgentChatsDir_ManagedWorktree_SiblingOfWorktree pins
// the git path (Task 7): a Crowbar-managed worktree (WorktreePath strictly under
// home) keeps its chats beside the worktree, so a workspace-root rm reaps them.
func TestAgentWorkspaceReader_AgentChatsDir_ManagedWorktree_SiblingOfWorktree(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "w1",
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: "/home/crowbar/projects/p1/github.com/acme/repo/feat-x/worktree",
		}},
		repos:       fakeRepoGetter{err: errors.New("managed path must not resolve a repo")},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	dir, err := r.AgentChatsDir(context.Background(), "w1")
	require.NoError(t, err)
	assert.Equal(t, "/home/crowbar/projects/p1/github.com/acme/repo/feat-x/chats", dir)
}

// TestAgentWorkspaceReader_AgentChatsDir_HomeKind_RerootsUnderHome pins the
// home-kind / adopted-checkout path (Task 7): a workspace whose WorktreePath is
// the user's REAL dir OUTSIDE home (the repo-home adopted at repo.Path) reroots its
// chats to <home>/projects/<proj>/<slug>/default/chats, so no plaintext ledger is
// ever written beside the user's repository. The slug is resolved from the repo's
// remote via worktreepath.RemoteSlug, exactly like worktree.resolveSlug.
func TestAgentWorkspaceReader_AgentChatsDir_HomeKind_RerootsUnderHome(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "wh",
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: "/Users/dev/my-real-repo", // repo.Path, OUTSIDE crowbar home
			IsDefault:    true,
		}},
		repos: fakeRepoGetter{repo: &domain.Repository{
			ID:        "r1",
			RemoteURL: "git@github.com:acme/repo.git",
		}},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	dir, err := r.AgentChatsDir(context.Background(), "wh")
	require.NoError(t, err)
	assert.Equal(t, "/home/crowbar/projects/p1/github.com/acme/repo/default/chats", dir)
}

// TestAgentWorkspaceReader_AgentChatsDir_ProjectHome_NoRepo_RerootsUnderProject
// covers the project-level home (WorkspaceKindHome, WorktreePath = project.Path
// outside home, NO repo id): it has no slug to resolve, so its chats still root
// strictly under home at <home>/projects/<proj>/default/chats — never beside the
// user's real project folder. This is exactly the case the brief's literal
// "resolve slug via RepoID" could not serve (no repo), so keying on the under-home
// test rather than Kind is what keeps it safe.
func TestAgentWorkspaceReader_AgentChatsDir_ProjectHome_NoRepo_RerootsUnderProject(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "wp",
			ProjectID:    "p1",
			Kind:         domain.WorkspaceKindHome,
			WorktreePath: "/Users/dev/my-project", // project.Path, OUTSIDE crowbar home
		}},
		repos:       fakeRepoGetter{err: errors.New("no repo must be looked up for a repo-less home")},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	dir, err := r.AgentChatsDir(context.Background(), "wp")
	require.NoError(t, err)
	assert.Equal(t, "/home/crowbar/projects/p1/default/chats", dir)
}

// TestAgentWorkspaceReader_AgentChatsDir_PoisonedSlug_FailsClosed proves the
// source guard (Task 7 safety): a crafted repo remote whose RemoteSlug carries
// "../" segments would let filepath.Join collapse the rerooted chats dir OUTSIDE
// crowbar home (a plaintext-ledger write onto the user's real filesystem).
// AgentChatsDir must fail CLOSED — return an error, never an escaping path.
func TestAgentWorkspaceReader_AgentChatsDir_PoisonedSlug_FailsClosed(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "wh",
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: "/Users/dev/my-real-repo", // OUTSIDE home → reroot path
		}},
		repos: fakeRepoGetter{repo: &domain.Repository{
			ID: "r1",
			// A hostile remote whose slug is host/../../../../etc → escapes home.
			RemoteURL: "git@evil.com:../../../../../../etc/cron.d.git",
		}},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	dir, err := r.AgentChatsDir(context.Background(), "wh")
	require.Error(t, err, "a chats dir that escapes crowbar home must fail closed")
	assert.Empty(t, dir)
	assert.Contains(t, err.Error(), "escapes crowbar home")
}

// TestAgentWorkspaceReader_AgentChatsDir_CrowbarHomeError proves a crowbarHome
// resolver failure short-circuits before any lookup.
func TestAgentWorkspaceReader_AgentChatsDir_CrowbarHomeError(t *testing.T) {
	wantErr := errors.New("no home")
	r := &agentWorkspaceReader{
		workspaces:  fakeWorkspaceGetter{err: errors.New("must not be reached")},
		crowbarHome: func() (string, error) { return "", wantErr },
	}

	_, err := r.AgentChatsDir(context.Background(), "w1")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

// TestAgentWorkspaceReader_AgentChatsDir_EmptyWorkspaceID_Errors documents
// exactly why chat.WorkspaceID may never drive a chat's own ledger path (spec
// §1.5): AgentChatsDir unconditionally resolves a workspace ROW, so a chat
// with no workspace — WorkspaceID == "" — has no row to find at all, not merely
// a "different" one. worktreepath.LedgerChatsDir (see the chat usecase's
// promptJournalDirFor) exists precisely so a ledger's own path never runs
// through this lookup.
func TestAgentWorkspaceReader_AgentChatsDir_EmptyWorkspaceID_Errors(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces:  fakeWorkspaceGetter{err: errors.New("workspace not found: \"\"")},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	_, err := r.AgentChatsDir(context.Background(), "")
	require.Error(t, err, "a chat with no workspace must not silently resolve SOME chats dir")
}
