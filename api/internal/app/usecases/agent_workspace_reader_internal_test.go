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

func TestAgentWorkspaceReader_WorktreeDir_Success(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "w1",
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: "/home/crowbar/projects/p1/r1/workspaces/w1/worktree",
			Provisioning: domain.WorkspaceProvisioned,
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

func TestAgentWorkspaceReader_AgentChatsDir_ManagedWorktree_SiblingOfWorktree(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "w1",
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: "/home/crowbar/projects/p1/github.com/acme/repo/feat-x/worktree",
			Provisioning: domain.WorkspaceProvisioned,
		}},
		repos:       fakeRepoGetter{err: errors.New("managed path must not resolve a repo")},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	dir, err := r.AgentChatsDir(context.Background(), "w1")
	require.NoError(t, err)
	assert.Equal(t, "/home/crowbar/projects/p1/github.com/acme/repo/feat-x/chats", dir)
}

func TestAgentWorkspaceReader_AgentChatsDir_HomeKind_RerootsUnderHome(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "wh",
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: "/Users/dev/my-real-repo",
			IsDefault:    true,
			Provisioning: domain.WorkspaceShared,
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

func TestAgentWorkspaceReader_AgentChatsDir_ProjectHome_NoRepo_RerootsUnderProject(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "wp",
			ProjectID:    "p1",
			Kind:         domain.WorkspaceKindHome,
			WorktreePath: "/Users/dev/my-project",
			Provisioning: domain.WorkspaceShared,
		}},
		repos:       fakeRepoGetter{err: errors.New("no repo must be looked up for a repo-less home")},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	dir, err := r.AgentChatsDir(context.Background(), "wp")
	require.NoError(t, err)
	assert.Equal(t, "/home/crowbar/projects/p1/default/chats", dir)
}

func TestAgentWorkspaceReader_AgentChatsDir_PoisonedSlug_FailsClosed(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "wh",
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: "/Users/dev/my-real-repo",
			Provisioning: domain.WorkspaceProvisioned,
		}},
		repos: fakeRepoGetter{repo: &domain.Repository{
			ID:        "r1",
			RemoteURL: "git@evil.com:../../../../../../etc/cron.d.git",
		}},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	dir, err := r.AgentChatsDir(context.Background(), "wh")
	require.Error(t, err, "a chats dir that escapes crowbar home must fail closed")
	assert.Empty(t, dir)
	assert.Contains(t, err.Error(), "escapes crowbar home")
}

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

func TestAgentWorkspaceReader_AgentChatsDir_EmptyWorkspaceID_Errors(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces:  fakeWorkspaceGetter{err: errors.New("workspace not found: \"\"")},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	_, err := r.AgentChatsDir(context.Background(), "")
	require.Error(t, err, "a chat with no workspace must not silently resolve SOME chats dir")
}

func TestAgentWorkspaceReader_AgentChatsDir_WorkspaceGetError(t *testing.T) {
	wantErr := errors.New("workspace not found")
	r := &agentWorkspaceReader{
		workspaces:  fakeWorkspaceGetter{err: wantErr},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	_, err := r.AgentChatsDir(context.Background(), "missing")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

func TestAgentWorkspaceReader_AgentChatsDir_RepoSlugErrorPropagates(t *testing.T) {
	wantErr := errors.New("read model unavailable")
	r := &agentWorkspaceReader{
		workspaces: fakeWorkspaceGetter{ws: domain.Workspace{
			ID:           "wh",
			ProjectID:    "p1",
			RepoID:       "r1",
			WorktreePath: "/Users/dev/my-real-repo",
			Provisioning: domain.WorkspaceProvisioned,
		}},
		repos:       fakeRepoGetter{err: wantErr},
		crowbarHome: func() (string, error) { return "/home/crowbar", nil },
	}

	_, err := r.AgentChatsDir(context.Background(), "wh")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

func TestAgentWorkspaceReader_RepoSlug_EmptyRepoIDYieldsEmptySlug(t *testing.T) {
	r := &agentWorkspaceReader{
		repos: fakeRepoGetter{err: errors.New("must not be looked up for a blank repo id")},
	}

	slug, err := r.repoSlug(context.Background(), "")
	require.NoError(t, err)
	assert.Empty(t, slug)
}

func TestAgentWorkspaceReader_RepoSlug_FindByKeyErrorPropagates(t *testing.T) {
	wantErr := errors.New("store down")
	r := &agentWorkspaceReader{repos: fakeRepoGetter{err: wantErr}}

	_, err := r.repoSlug(context.Background(), "r1")
	require.Error(t, err)
	assert.ErrorIs(t, err, wantErr)
}

func TestAgentWorkspaceReader_RepoSlug_UnknownRepoIsAnError(t *testing.T) {
	r := &agentWorkspaceReader{repos: fakeRepoGetter{repo: nil}}

	_, err := r.repoSlug(context.Background(), "does-not-exist")
	require.Error(t, err)
	assert.Contains(t, err.Error(), `repo "does-not-exist" not found`)
}
