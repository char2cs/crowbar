package hierarchy_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestCreateFromImport_ResolveHolderPath_CrowbarHomeError proves that when a
// branch's real create fails AND resolveHolderPath cannot even resolve crowbar
// home (needed to classify a holder as managed-vs-external), the placeholder
// fallback still records a row with no holder path — the same degrade as a
// genuinely free branch — rather than failing the whole import.
func TestCreateFromImport_ResolveHolderPath_CrowbarHomeError(t *testing.T) {
	var placeholder workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			placeholder = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	g := &fakeGit{}
	badHome := func() (string, error) { return "", errBoom }
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), badHome)
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: []string{"feat/x"},
	})

	require.NoError(t, err, "an unresolvable crowbar home must still yield a placeholder row, not a stranded batch")
	assert.Empty(t, placeholder.WorktreePath, "the placeholder signal is an empty worktree path")
	assert.Empty(t, placeholder.HeldByPath, "with no crowbar home to classify against, there is no holder to name")
}

// TestCreateFromImport_ResolveHolderPath_HolderResolveError proves the same
// degrade when resolveHolderPath's own holder.Resolve call fails (here, a
// broken `git worktree list`): the placeholder is still created with no
// holder path, and the import batch is not aborted.
func TestCreateFromImport_ResolveHolderPath_HolderResolveError(t *testing.T) {
	var placeholder workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			placeholder = in
			return domain.Workspace{ID: in.ID}, nil
		},
	}
	g := &fakeGit{addErr: errBoom, worktreeListErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{}, newNow(), fakeHome())
	withOwningChats(uc)

	err := uc.CreateFromImport(context.Background(), hierarchy.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo",
		RemoteURL: "https://github.com/test/repo.git", DefaultBranch: "main",
		Branches: []string{"feat/x"},
	})

	require.NoError(t, err, "a broken holder-resolve read must still yield a placeholder row")
	assert.Empty(t, placeholder.WorktreePath)
	assert.Empty(t, placeholder.HeldByPath, "an unresolvable holder is reported the same as a free branch")
}
