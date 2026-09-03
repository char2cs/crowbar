package worktree_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineprovider "github.com/char2cs/crowbar/api/internal/engine/provider"
)

// TestPrBaseGraph_FiltersLinksWithoutHeadOrBase proves the open-PR graph only
// keeps edges with BOTH a head and a base, and that a resolved edge correctly
// drives an extra ancestor's creation and the requested branch's parenting —
// distinct from a provider FAILURE (covered elsewhere), this is the graph's
// own per-edge validation on an otherwise-successful response.
func TestPrBaseGraph_FiltersLinksWithoutHeadOrBase(t *testing.T) {
	var created []workspace.CreateInput
	ws := &fakeWorkspace{
		CreateFn: func(_ context.Context, in workspace.CreateInput, _ time.Time) (domain.Workspace, error) {
			created = append(created, in)
			return domain.Workspace{ID: "new-" + in.Branch, Branch: in.Branch}, nil
		},
	}
	g := &fakeGit{addStartSha: "sha"}
	provider := &fakeProvider{prLinks: []engineprovider.PRLink{
		{Head: "feature/x", Base: "develop"}, // the one valid edge
		{Head: "", Base: "y"},                // no head: must be ignored
		{Head: "z", Base: ""},                // no base: must be ignored
	}}
	uc := worktree.New(ws, g, provider, &fakeRepoStore{}, newNow(), fakeHome())

	err := uc.CreateFromImport(context.Background(), worktree.ImportInput{
		RepoID: "r1", ProjectID: "p1", RepoPath: "/repo", DefaultBranch: "main",
		Branches: []string{"feature/x"},
	})
	require.NoError(t, err)
	require.Len(t, created, 2, "the missing ancestor 'develop' must be created too")
	assert.Equal(t, "develop", created[0].Branch, "ancestors are created before their children")
	assert.Empty(t, created[0].ParentID, "develop has no PR of its own, so it parents at the default")
	assert.Equal(t, "feature/x", created[1].Branch)
	assert.Equal(t, "new-develop", created[1].ParentID, "feature/x nests under the freshly created develop")
}
