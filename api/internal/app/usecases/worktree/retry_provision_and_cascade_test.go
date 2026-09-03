package worktree_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestRetryProvision_OriginBranch_FetchFails_StillChecksOutBestEffort proves
// originHasBranch's own best-effort contract: a failed refresh of origin/<branch>
// must not stop the branch from being treated as "on origin" when the LOCAL
// remote-tracking ref already resolves it — the checkout still runs AT that ref
// rather than falling back to a plain local-branch checkout.
func TestRetryProvision_OriginBranch_FetchFails_StillChecksOutBestEffort(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	g := &fakeGit{trackingExists: true, fetchRefErr: errBoom, revParseSha: "originsha"}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err, "a failed origin refresh must not block checkout from the local remote-tracking ref")
	assert.Contains(t, g.ops(), "WorktreeAddAtRef", "still checks out AT origin's ref despite the failed refresh")
	assert.NotContains(t, g.ops(), "WorktreeAdd", "must not silently fall back to a plain local checkout")
}

// TestRetryProvision_LocalBranch_ForkPointUnresolved_StillSucceeds proves the
// fork point read in materializeProtectedWorktree's LOCAL-branch path is
// non-essential: when it fails, provisioning still succeeds with an empty fork
// point rather than failing a checkout that already landed cleanly on disk.
func TestRetryProvision_LocalBranch_ForkPointUnresolved_StillSucceeds(t *testing.T) {
	var gotSha string
	sawSha := false
	ws := placeholderWS("ph", "r1", "p1", "develop")
	ws.ProvisionInPlaceFn = func(id, path, sha string) (domain.Workspace, error) {
		gotSha, sawSha = sha, true
		return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha}, nil
	}
	g := &fakeGit{revParseErr: errBoom} // no tracking ref -> local WorktreeAdd path
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err, "an unresolved fork point is non-essential and must not fail provisioning")
	require.True(t, sawSha, "ProvisionInPlace must still be reached")
	assert.Empty(t, gotSha, "the fork point is simply left empty when RevParse cannot resolve it")
}

// TestDeleteRepoWorkspaces_SkipsChildAlreadyReachedByParentsCascade proves the
// dedup guard: a workspace whose parent is ALSO one of the repo's own rows is not
// walked as its own separate root — it is only reached once, via the parent's
// cascade. Without the guard it would be deleted a second time as an unrelated
// root, which would emit a second tombstone for an aggregate that is already gone.
func TestDeleteRepoWorkspaces_SkipsChildAlreadyReachedByParentsCascade(t *testing.T) {
	var deleted []string
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "root", RepoID: "r1", ProjectID: "p1", WorktreePath: "/wt/root", Branch: "b-root"},
				{ID: "child", RepoID: "r1", ProjectID: "p1", ParentID: "root", WorktreePath: "/wt/child", Branch: "b-child"},
			}, nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	g := &fakeGit{}
	uc := worktree.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	handled, err := uc.DeleteRepoWorkspaces(context.Background(), "r1", "/repo")
	require.NoError(t, err)
	assert.Equal(t, []string{"child", "root"}, handled, "the child is reached exactly once, via the root's own cascade")
	assert.Len(t, deleted, 2, "without the skip, the child would be deleted again as a spurious second root")
}
