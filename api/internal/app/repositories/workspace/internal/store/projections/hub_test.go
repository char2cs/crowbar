package projections

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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

// TestRegisterHub_ProjectionFrameMatchesDirectRebroadcast asserts the two
// triggers that must emit an identical WS frame — the event-driven hub
// projection and the request-bracketed BeginWork/EndWork rebroadcast — converge
// on the SAME enrich+broadcast, so the FE spinner and merge badges are
// consistent regardless of which path fired (spec §3.5 hub-frame enrichment).
func TestRegisterHub_ProjectionFrameMatchesDirectRebroadcast(t *testing.T) {
	ctx, ax, st := newRegistered(t)

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

	RegisterHub(st, enrich, broadcast)

	// SendWait blocks until every matching projection handler completes, so the
	// hub projection's broadcast has fired by the time it returns.
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "main", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
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

// A frame is only ever sent for state the read model already holds: a client
// that re-reads the model on a frame must never get older state back. (A hub
// subscribed beside the store projection ran concurrently with its save.)
func TestRegisterHub_NeverAnnouncesAheadOfTheReadModel(t *testing.T) {
	ctx, ax, st := newRegistered(t)
	var (
		mu    sync.Mutex
		ahead []string
	)
	RegisterHub(st,
		func(_ context.Context, ws domain.Workspace) domain.Workspace { return ws },
		func(ws domain.Workspace) {
			row, err := st.Get(ctx, ws.ID)
			if err != nil || row == nil || row.Branch != ws.Branch {
				mu.Lock()
				ahead = append(ahead, ws.Branch)
				mu.Unlock()
			}
		})
	_, err := ax.SendWait(ctx, wscmds.CreateWorkspace{
		ID: "w1", RepoID: "r1", ProjectID: "p1", Branch: "b-0", Now: time.Unix(1, 0).UTC(),
		Provisioning: domain.WorkspacePlaceholder,
	})
	require.NoError(t, err)
	for i := 1; i <= 50; i++ {
		_, err := ax.SendWait(ctx, wscmds.RenameBranch{ID: "w1", Branch: fmt.Sprintf("b-%d", i)})
		require.NoError(t, err)
	}
	mu.Lock()
	defer mu.Unlock()
	assert.Empty(t, ahead, "frames sent before the read model held their state")
}
