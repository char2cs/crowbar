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

// TestAgentWorkspaceReader_WorktreeDir_Success proves WorktreeDir resolves the
// owning project/repo from the workspace repo and derives the git worktree
// directory via worktreepath.For, rooted at the injected crowbarHome (not
// metadata.GetHomePath, so it never diverges from a hermetic test's overridden
// home).
func TestAgentWorkspaceReader_WorktreeDir_Success(t *testing.T) {
	r := &agentWorkspaceReader{
		workspaces:  fakeWorkspaceGetter{ws: domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}},
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
