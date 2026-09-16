package project_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/project"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestProjectDelete_RealFilesystem_LeavesNoResidue is the end-to-end proof
// this plan exists for: after Delete() returns, the project's ENTIRE on-disk
// tree — not just its icon, not just one repo's worktree, everything under
// projects/<id>/ — is gone. No mocked RemoveAll: this runs the real default
// against a real temp directory populated the way an actual project's tree
// looks (an icon file, a repo subdirectory holding a worktree with real
// files in it), which is exactly the shape a bare "no error was returned"
// unit test can't catch a regression in.
func TestProjectDelete_RealFilesystem_LeavesNoResidue(t *testing.T) {
	home := t.TempDir()
	projectDir := filepath.Join(home, "projects", "p1")
	repoDir := filepath.Join(projectDir, "r1")
	worktreeDir := filepath.Join(repoDir, "feature-x", "worktree")
	require.NoError(t, os.MkdirAll(worktreeDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "icon"), []byte("fake-png-bytes"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "README.md"), []byte("hi"), 0o644))

	f := newDeleteFixture(t)
	f.seedProject()
	f.workspaces.workspaces = []domain.Workspace{
		{
			ID: "w-child", RepoID: "r1", ProjectID: "p1",
			Branch: "feature-x", WorktreePath: worktreeDir,
		},
	}
	f.uc = project.NewDelete(project.DeleteDeps{
		Projects:    f.projects,
		Repos:       f.repos,
		Workspaces:  f.workspaces,
		Git:         f.git,
		CrowbarHome: func() (string, error) { return home, nil },
		// RemoveAll deliberately left unset: this test exercises the REAL
		// os.RemoveAll default, not a stub.
	})

	require.NoError(t, f.uc.Delete(context.Background(), "p1"))

	_, err := os.Stat(projectDir)
	require.True(t, os.IsNotExist(err), "projects/p1 must not exist at all after Delete(), got err=%v", err)
}
