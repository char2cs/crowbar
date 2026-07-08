package projections

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	wscmds "github.com/char2cs/crowbar/api/internal/app/repositories/workspace/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// stubFrame stands in for the api-layer wire frame the container supplies in
// production (dto.WorkspaceDTO). It carries the base aggregate fields plus the
// two derived overlays (Working, CanMergeLocally) that are NOT part of
// evt.Aggregate and are attached by the injected enrich callback (spec §3.5).
type stubFrame struct {
	ID              string                 `json:"id"`
	Branch          string                 `json:"branch"`
	ProjectID       string                 `json:"projectId"`
	RepoID          string                 `json:"repoId"`
	Status          domain.WorkspaceStatus `json:"status"`
	Working         bool                   `json:"working"`
	CanMergeLocally bool                   `json:"canMergeLocally"`
}

// newHubAsynx builds a real asynx over a temp in-memory event store — the
// production shape (one singleton), driven over a throwaway DB.
func newHubAsynx(
	t *testing.T,
) (context.Context, asynx.Asynx[domain.Workspace]) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Workspace]().
		WithEventStore(es).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	return context.Background(), ax
}

// TestRegisterHub_ProjectionFrameMatchesDirectRebroadcast asserts the two
// triggers that must emit an identical WS frame — the event-driven hub
// projection and the request-bracketed BeginWork/EndWork rebroadcast — converge
// on the SAME enrich+broadcast, so the FE spinner and merge badges are
// consistent regardless of which path fired (spec §3.5 hub-frame enrichment).
func TestRegisterHub_ProjectionFrameMatchesDirectRebroadcast(t *testing.T) {
	ctx, ax := newHubAsynx(t)

	var (
		mu      sync.Mutex
		frames  []stubFrame
		lastAgg domain.Workspace
	)
	// enrich attaches the derived overlays (Working, CanMergeLocally) that are NOT
	// part of evt.Aggregate — the same callback both the hub projection and the
	// BeginWork/EndWork rebroadcast share in production.
	enrich := func(_ context.Context, ws domain.Workspace) stubFrame {
		mu.Lock()
		lastAgg = ws
		mu.Unlock()
		return stubFrame{
			ID:              ws.ID,
			Branch:          ws.Branch,
			ProjectID:       ws.ProjectID,
			RepoID:          ws.RepoID,
			Status:          ws.Status,
			Working:         true,
			CanMergeLocally: true,
		}
	}
	broadcast := func(f stubFrame) {
		mu.Lock()
		frames = append(frames, f)
		mu.Unlock()
	}

	require.NoError(t, RegisterHub(ax, enrich, broadcast))

	// SendWait blocks until every matching projection handler completes, so the
	// hub projection's broadcast has fired by the time it returns.
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
	})
	require.NoError(t, err)

	mu.Lock()
	require.Len(t, frames, 1, "the hub projection must broadcast exactly one frame for the create event")
	projFrame := frames[0]
	agg := lastAgg
	mu.Unlock()

	// The base frame is derived from evt.Aggregate...
	assert.Equal(t, "w1", projFrame.ID)
	assert.Equal(t, "main", projFrame.Branch)
	assert.Equal(t, "p1", projFrame.ProjectID)
	assert.Equal(t, "r1", projFrame.RepoID)
	assert.Equal(t, domain.WorkspaceStatusNew, projFrame.Status)
	// ...plus the derived overlays the injected enrich attached, proving
	// enrichment ran (spec §3.5).
	assert.True(t, projFrame.Working, "enrich must attach the Working overlay")
	assert.True(t, projFrame.CanMergeLocally, "enrich must attach the merge-eligibility overlay")

	// The BeginWork-style rebroadcast runs the SAME enrich+broadcast directly on
	// the SAME aggregate (it fires on the 202 ack, not on an event). The emitted
	// frame must be byte-identical to the hub projection's.
	broadcast(enrich(ctx, agg))
	mu.Lock()
	require.Len(t, frames, 2)
	directFrame := frames[1]
	mu.Unlock()

	assert.Equal(t, projFrame, directFrame)
	pj, err := json.Marshal(projFrame)
	require.NoError(t, err)
	dj, err := json.Marshal(directFrame)
	require.NoError(t, err)
	assert.Equal(t, string(pj), string(dj), "hub-projection frame and BeginWork rebroadcast frame must be byte-identical")
}

func TestRegisterHub_SubscribeError(t *testing.T) {
	err := RegisterHub(
		&fakeAx{subscribeErr: errors.New("bus down")},
		func(context.Context, domain.Workspace) stubFrame { return stubFrame{} },
		func(stubFrame) {},
	)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "workspace hub projection: subscribe")
}
