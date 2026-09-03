package worktree_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestCreateChild_AdoptMainWorktree_ResolveLockedError proves a provider
// failure while resolving whether the ADOPTED main worktree's branch is
// protected surfaces wrapped — distinct from a RevParse(HEAD) failure, which a
// sibling test already covers — and never persists a row.
func TestCreateChild_AdoptMainWorktree_ResolveLockedError(t *testing.T) {
	g := &fakeGit{revParseSha: "headsha"}
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) { return nil, nil },
		CreateFn: func(_ context.Context, _ workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			t.Fatal("Create must not run when resolving the locked flag fails")
			return domain.Workspace{}, nil
		},
	}
	uc := worktree.New(ws, g, &fakeProvider{err: errBoom}, &fakeRepoStore{}, newNow(), fakeHome())

	_, err := uc.CreateChild(context.Background(), worktree.CreateChildInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", Branch: "main", ParentBranch: "main",
	})

	require.ErrorIs(t, err, errBoom)
}
