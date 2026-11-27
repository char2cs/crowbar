package hierarchy_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace/internal/hierarchy"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestRetryProvision_OriginBranch_FetchFails_StillChecksOutBestEffort(t *testing.T) {
	ws := placeholderWS("ph", "r1", "p1", "develop")
	g := &fakeGit{trackingExists: true, fetchRefErr: errBoom, revParseSha: "originsha"}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err, "a failed origin refresh must not block checkout from the local remote-tracking ref")
	assert.Contains(t, g.ops(), "WorktreeAddAtRef", "still checks out AT origin's ref despite the failed refresh")
	assert.NotContains(t, g.ops(), "WorktreeAdd", "must not silently fall back to a plain local checkout")
}

func TestRetryProvision_LocalBranch_ForkPointUnresolved_StillSucceeds(t *testing.T) {
	var gotSha string
	sawSha := false
	ws := placeholderWS("ph", "r1", "p1", "develop")
	ws.ProvisionInPlaceFn = func(id, path, sha string) (domain.Workspace, error) {
		gotSha, sawSha = sha, true
		return domain.Workspace{ID: id, WorktreePath: path, ForkPointSha: sha, Provisioning: domain.WorkspaceProvisioned}, nil
	}
	g := &fakeGit{revParseErr: errBoom}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	_, err := uc.RetryProvision(context.Background(), "ph")
	require.NoError(t, err, "an unresolved fork point is non-essential and must not fail provisioning")
	require.True(t, sawSha, "ProvisionInPlace must still be reached")
	assert.Empty(t, gotSha, "the fork point is simply left empty when RevParse cannot resolve it")
}

func TestDeleteRepoWorkspaces_SkipsChildAlreadyReachedByParentsCascade(t *testing.T) {
	var deleted []string
	ws := &fakeWorkspace{
		ListFn: func(_ context.Context) ([]domain.Workspace, error) {
			return []domain.Workspace{
				{ID: "root", RepoID: "r1", ProjectID: "p1", WorktreePath: "/wt/root", Branch: "b-root", Provisioning: domain.WorkspaceProvisioned},
				{ID: "child", RepoID: "r1", ProjectID: "p1", ParentID: "root", WorktreePath: "/wt/child", Branch: "b-child", Provisioning: domain.WorkspaceProvisioned},
			}, nil
		},
		DeleteFn: func(_ context.Context, id string) error {
			deleted = append(deleted, id)
			return nil
		},
	}
	g := &fakeGit{}
	uc := hierarchy.New(ws, g, &fakeProvider{}, &fakeRepoStore{path: "/repo"}, newNow(), fakeHome())

	err := uc.DeleteRepoWorkspaces(context.Background(), domain.Repository{ID: "r1", Path: "/repo"}, domain.KeepWorkAtRisk)
	require.NoError(t, err)
	assert.Equal(t, []string{"child", "root"}, deleted,
		"the child is reached exactly once, via the root's own cascade — without the skip it "+
			"would be deleted again as a spurious second root")
}
