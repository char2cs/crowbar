package workspace_test

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/app/repositories/workspace"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// appendV2 writes one schema-2 event carrying patches onto id's history.
func appendV2(t *testing.T, es asynxModels.Store, id, patches string) {
	t.Helper()
	ctx := context.Background()
	prior, err := es.ReadFrom(ctx, "events:"+id, 1)
	require.NoError(t, err)
	v := int64(len(prior) + 1)
	evt, err := json.Marshal(map[string]any{
		"id": fmt.Sprintf("v2-%s-%d", id, v), "event_name": "workspace.last_error_set." + id, "version": v,
		"schema_version": 2, "occurred_at": time.Unix(2, 0).UTC(), "patches": json.RawMessage(patches),
	})
	require.NoError(t, err)
	require.NoError(t, es.Atomically(ctx, func(tx asynxModels.Tx) error { return tx.Append(ctx, "events:"+id, v, evt) }))
}

// upcastingRepo is buildRepo with the upcasters production registers.
func upcastingRepo(t *testing.T, ad *adapter.Container) workspace.Workspace {
	t.Helper()
	rm, err := workspace.OpenReadModel(ad.WorkspaceView())
	require.NoError(t, err)
	b := asynx.New[domain.Workspace]().WithEventStore(ad.WorkspaceES()).WithSnapshotStore(asynxstore.NewSnapshots())
	workspace.RetiredFields(b)
	workspace.Subscriber(rm)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	repo, err := workspace.New(ax, ad.WorkspaceES(), rm)
	require.NoError(t, err)
	return repo
}

// History written while workspaces carried lastError still sets, clears and
// replaces it; replayed against a struct without the key, it fails every read.
func TestWorkspace_AV2HistoryThatPatchesLastErrorStillReplays(t *testing.T) {
	ad := newAdapter(t, t.TempDir())
	repo := upcastingRepo(t, ad)
	workspace.WaitQuiescentForTest(repo)
	ctx := context.Background()
	_, err := repo.Create(ctx, workspace.CreateInput{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Provisioning: domain.WorkspacePlaceholder,
	}, time.Unix(1, 0))
	require.NoError(t, err)
	// A fold that starts after a snapshot of the old struct has no key to
	// replace or remove: the first two patches are what that history looks like.
	for _, patches := range []string{
		`[{"op":"replace","path":"/lastError","value":"second"}]`,
		`[{"op":"remove","path":"/lastError"}]`,
		`[{"op":"add","path":"/lastError","value":"again"},{"op":"replace","path":"/branch","value":"feat/a"}]`,
	} {
		appendV2(t, ad.WorkspaceES(), "w1", patches)
	}

	got, err := upcastingRepo(t, ad).Get(ctx, "w1")

	require.NoError(t, err)
	assert.Equal(t, "feat/a", got.Branch, "the rest of each patch still applies")
	assert.Equal(t, domain.WorkspacePlaceholder, got.Provisioning)
}
