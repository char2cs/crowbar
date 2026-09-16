package node_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	nodecmds "github.com/char2cs/crowbar/api/internal/app/repositories/node"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// captureBroadcast records every (nodeID, parentID, kind) frame the hub
// projection fans out, mirroring agentchat's own test capture.
type captureBroadcast struct {
	mu   sync.Mutex
	rows []string
}

func (c *captureBroadcast) watch(e nodecmds.NodeEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, fmt.Sprintf("%s:%s:%s", e.NodeID, e.ParentID, e.Kind))
}

func (c *captureBroadcast) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.rows)
}

// newRepoWithDeps builds an EventStore repo over throwaway in-memory event +
// read-model DBs and returns the read-model DB and the captured hub broadcast
// so tests can inspect persistence + fan-out.
func newRepoWithDeps(
	t *testing.T,
) (context.Context, nodecmds.EventStore, *gormdb.DB, *captureBroadcast) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Node]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	cap := &captureBroadcast{}
	repo, err := nodecmds.NewEventSourced(ax, es, db, cap.watch)
	require.NoError(t, err)
	return context.Background(), repo, db, cap
}

func newRepo(
	t *testing.T,
) (context.Context, nodecmds.EventStore) {
	t.Helper()
	ctx, repo, _, _ := newRepoWithDeps(t)
	return ctx, repo
}

func createNode(
	t *testing.T,
	ctx context.Context,
	repo nodecmds.EventStore,
	id string,
	kind domain.NodeKind,
	parentID string,
	order int,
) domain.Node {
	t.Helper()
	n, err := repo.Create(ctx, id, kind, parentID, order)
	require.NoError(t, err)
	return n
}

func TestNode_CreateAndGetNode(t *testing.T) {
	ctx, repo := newRepo(t)
	created := createNode(t, ctx, repo, "n1", domain.NodeKindChat, "", 0)
	assert.Equal(t, "n1", created.ID)
	assert.Equal(t, domain.NodeKindChat, created.Kind)

	// Create is the one command on the SendWait path, so the read model already
	// reflects it.
	got, err := repo.GetNode(ctx, "n1")
	require.NoError(t, err)
	assert.Equal(t, domain.NodeKindChat, got.Kind)
}

func TestNode_Create_ErrorOnDuplicate(t *testing.T) {
	ctx, repo := newRepo(t)
	createNode(t, ctx, repo, "n1", domain.NodeKindChat, "", 0)

	_, err := repo.Create(ctx, "n1", domain.NodeKindChat, "", 0)
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestNode_GetNode_MissingBridgesToPackageErrNotFound(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.GetNode(ctx, "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, nodecmds.ErrNotFound)
}

// TestNode_SetPlacement_ChangesBothFields proves SetPlacement moves a node's
// parent AND its index, and the write is visible through GetNode once the
// projection settles.
func TestNode_SetPlacement_ChangesBothFields(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)
	createNode(t, ctx, repo, "n1", domain.NodeKindFolder, "", 0)
	createNode(t, ctx, repo, "n2", domain.NodeKindChat, "", 0)

	require.NoError(t, repo.SetPlacement(ctx, "n2", "n1", 3))
	nodecmds.WaitQuiescentForTest(repo)

	got, err := repo.GetNode(ctx, "n2")
	require.NoError(t, err)
	assert.Equal(t, "n1", got.ParentID)
	assert.Equal(t, 3, got.Order)
}

// TestNode_SetOrder_ChangesOnlyOrder pins the split at the repository boundary
// — the exact property the whole Node aggregate exists to guarantee (2026-09-08
// sidebar-placement-unification design §2.2): a renumber (SetOrder) cannot
// express a parent at all, because the command applies it to the node folded
// from the log, so the row keeps the parent the log has whatever the caller
// believed when it planned the renumber. This is the precise fix for the
// production bug — a dragged repo silently snapping back to position 0 — that
// motivated building Node as one shared aggregate in the first place.
func TestNode_SetOrder_ChangesOnlyOrder(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)
	createNode(t, ctx, repo, "n1", domain.NodeKindFolder, "", 0)
	createNode(t, ctx, repo, "n2", domain.NodeKindChat, "", 0)
	require.NoError(t, repo.SetPlacement(ctx, "n2", "n1", 0))

	require.NoError(t, repo.SetOrder(ctx, "n2", 3))
	nodecmds.WaitQuiescentForTest(repo)

	got, err := repo.GetNode(ctx, "n2")
	require.NoError(t, err)
	assert.Equal(t, "n1", got.ParentID, "the node is still threaded off the one it was filed under")
	assert.Equal(t, 3, got.Order)
}

func TestNode_SetOrder_RefusesANodeThatDoesNotExist(t *testing.T) {
	ctx, repo := newRepo(t)

	err := repo.SetOrder(ctx, "no-such-node", 0)
	assert.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestNode_SetPlacement_RefusesANodeThatDoesNotExist(t *testing.T) {
	ctx, repo := newRepo(t)

	err := repo.SetPlacement(ctx, "no-such-node", "", 0)
	assert.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

// TestNode_ListByParent_ReturnsSiblings proves ListByParent returns every node
// filed under a parent, with no particular ordering guaranteed — the caller
// densifies.
func TestNode_ListByParent_ReturnsSiblings(t *testing.T) {
	ctx, repo := newRepo(t)
	createNode(t, ctx, repo, "folder1", domain.NodeKindFolder, "", 0)
	createNode(t, ctx, repo, "n1", domain.NodeKindChat, "folder1", 0)
	createNode(t, ctx, repo, "n2", domain.NodeKindChat, "folder1", 1)
	createNode(t, ctx, repo, "n3", domain.NodeKindWorkspace, "elsewhere", 0)

	siblings, err := repo.ListByParent(ctx, "folder1")
	require.NoError(t, err)
	got := map[string]bool{}
	for _, n := range siblings {
		got[n.ID] = true
	}
	assert.Len(t, siblings, 2)
	assert.True(t, got["n1"])
	assert.True(t, got["n2"])
	assert.False(t, got["n3"])

	none, err := repo.ListByParent(ctx, "no-such-parent")
	require.NoError(t, err)
	assert.Empty(t, none)
}

// TestNode_Forget_ErasesAggregate mirrors agentchat's
// TestAgentChat_Forget_ErasesAggregate: Forget hard-deletes via ax.Forget — the
// synchronous OnForget drops the read-model row AND the underlying event log is
// erased, so a subsequent GetNode cannot self-heal it back via lazy Replay and
// genuinely reports node.ErrNotFound.
func TestNode_Forget_ErasesAggregate(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	createNode(t, ctx, repo, "n1", domain.NodeKindChat, "", 0)
	nodecmds.WaitQuiescentForTest(repo)

	require.NoError(t, repo.Forget(ctx, "n1"))
	nodecmds.WaitQuiescentForTest(repo)

	var n int64
	require.NoError(t, db.WithContext(ctx).Table("nodes_read").Count(&n).Error)
	assert.Zero(t, n, "Forget's OnForget must drop the read-model row")

	_, err := repo.GetNode(ctx, "n1")
	require.Error(t, err)
	assert.ErrorIs(t, err, nodecmds.ErrNotFound)
}

func TestNode_Forget_RefusesANodeThatDoesNotExist(t *testing.T) {
	ctx, repo := newRepo(t)
	err := repo.Forget(ctx, "no-such-node")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestNode_HubBroadcastsNodeIDAndKind(t *testing.T) {
	ctx, repo, _, cap := newRepoWithDeps(t)
	createNode(t, ctx, repo, "n1", domain.NodeKindChat, "", 0)

	nodecmds.WaitQuiescentForTest(repo)
	assert.GreaterOrEqual(t, cap.count(), 1, "hub projection must broadcast a lifecycle frame")
}

// TestNode_HubToleratesANilWatch proves a store built with no watch (production
// until a live-update consumer is wired, and every unit test that doesn't care
// about broadcast) never panics — see internal/store/hub.go's emit doc.
func TestNode_HubToleratesANilWatch(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Node]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)

	repo, err := nodecmds.NewEventSourced(ax, es, db, nil)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		_, err := repo.Create(context.Background(), "n1", domain.NodeKindChat, "", 0)
		require.NoError(t, err)
	})
}

// concurrentCreateTrials is the number of independent trials
// TestNode_ConcurrentCreate_OneWins runs, mirroring agentchat's own race test.
const concurrentCreateTrials = 10

// TestNode_ConcurrentCreate_OneWins exercises the OCC retry contract (no
// per-aggregate writeMu) under a REAL race: two Creates of the SAME id, fired
// concurrently via errgroup. Exactly one commits; the loser is rejected by
// Validate ("exists", wrapping asynxModels.ErrValidation) rather than left as
// an unresolved ErrPipelineFailed.
func TestNode_ConcurrentCreate_OneWins(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)

	for trial := range concurrentCreateTrials {
		nodeID := fmt.Sprintf("race-node-%d", trial)

		results := make([]error, 2)
		var g errgroup.Group
		for i := range results {
			g.Go(func() error {
				_, results[i] = repo.Create(ctx, nodeID, domain.NodeKindChat, "", 0)
				return nil
			})
		}
		require.NoError(t, g.Wait())

		succeeded := 0
		for _, e := range results {
			if e == nil {
				succeeded++
				continue
			}
			require.ErrorIsf(t, e, asynxModels.ErrValidation,
				"trial %d: a losing Create must be rejected by Validate, not left as an unresolved pipeline failure", trial)
		}
		require.Equalf(t, 1, succeeded, "trial %d: exactly one racing Create must commit; the other must be rejected", trial)

		nodecmds.WaitQuiescentForTest(repo)
		got, err := repo.GetNode(ctx, nodeID)
		require.NoError(t, err)
		require.Equalf(t, domain.NodeKindChat, got.Kind, "trial %d: the read model must hold exactly one node for the id", trial)
	}
}

// TestNode_OccSendErrorDisposition pins the terminal error-disposition contract
// against a fake send: ErrPipelineFailed is retried exactly MaxOCCAttempts then
// surfaced (→ 409); ErrValidation is never retried (→ 422); ErrQueueFull is
// never retried and is translated to apperr.ErrUnavailable (→ 503). All
// classified via errors.Is. Mirrors agentchat's own OCC disposition test.
func TestNode_OccSendErrorDisposition(t *testing.T) {
	ctx := context.Background()
	cmd := setOrderCmd{}

	t.Run("ErrPipelineFailed retried then surfaced", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Node]) (asynxModels.Event[domain.Node], error) {
			calls++
			return asynxModels.Event[domain.Node]{}, fmt.Errorf("boom: %w", asynxModels.ErrPipelineFailed)
		}
		_, err := nodecmds.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrPipelineFailed)
		assert.Equal(t, nodecmds.MaxOCCAttempts, calls)
	})

	t.Run("ErrValidation never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Node]) (asynxModels.Event[domain.Node], error) {
			calls++
			return asynxModels.Event[domain.Node]{}, fmt.Errorf("nope: %w", asynxModels.ErrValidation)
		}
		_, err := nodecmds.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, asynxModels.ErrValidation)
		assert.Equal(t, 1, calls)
	})

	t.Run("ErrQueueFull translated to unavailable, never retried", func(t *testing.T) {
		calls := 0
		send := func(context.Context, asynxModels.Command[domain.Node]) (asynxModels.Event[domain.Node], error) {
			calls++
			return asynxModels.Event[domain.Node]{}, fmt.Errorf("full: %w", asynxModels.ErrQueueFull)
		}
		_, err := nodecmds.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, apperr.ErrUnavailable)
		assert.Equal(t, 1, calls)
	})

	t.Run("success returns immediately", func(t *testing.T) {
		send := func(_ context.Context, c asynxModels.Command[domain.Node]) (asynxModels.Event[domain.Node], error) {
			return asynxModels.Event[domain.Node]{Aggregate: domain.Node{ID: c.AggregateID()}}, nil
		}
		evt, err := nodecmds.OccSend(ctx, send, cmd)
		require.NoError(t, err)
		assert.Equal(t, "n1", evt.Aggregate.ID)
	})

	t.Run("unclassified error surfaced as-is, not retried", func(t *testing.T) {
		calls := 0
		sentinel := errors.New("driver: connection reset")
		send := func(context.Context, asynxModels.Command[domain.Node]) (asynxModels.Event[domain.Node], error) {
			calls++
			return asynxModels.Event[domain.Node]{}, sentinel
		}
		_, err := nodecmds.OccSend(ctx, send, cmd)
		require.ErrorIs(t, err, sentinel)
		assert.Equal(t, 1, calls, "an unclassified error must not be retried")
	})
}

// setOrderCmd is a minimal asynxModels.Command[domain.Node] fake for
// TestNode_OccSendErrorDisposition, which only needs AggregateID.
type setOrderCmd struct{}

func (setOrderCmd) AggregateID() string         { return "n1" }
func (setOrderCmd) EventName() string           { return "node.order_set.n1" }
func (setOrderCmd) ShouldSnapshot() bool        { return false }
func (setOrderCmd) Validate(*domain.Node) error { return nil }
func (setOrderCmd) EmitEvent(current *domain.Node) domain.Node {
	if current == nil {
		return domain.Node{}
	}
	return *current
}

func TestNode_NewEventSourced_ErrorOnBadDB(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := asynx.New[domain.Node]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000}).
		Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = nodecmds.NewEventSourced(ax, es, db, func(nodecmds.NodeEvent) {})
	require.Error(t, err)
}

// A read that FAILS is not a read that missed.
func TestNode_GetNode_AReadFailureIsNotAMiss(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)

	createNode(t, ctx, repo, "n1", domain.NodeKindChat, "", 0)
	nodecmds.WaitQuiescentForTest(repo)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.GetNode(ctx, "n1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, nodecmds.ErrNotFound,
		"a node that cannot be READ is not a node that is gone")
}

// TestNode_ListByParent_PropagatesStorageFailure proves ListByParent behaves
// like GetNode: a read-model that cannot be READ must surface as an error,
// never as a silently empty list.
func TestNode_ListByParent_PropagatesStorageFailure(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	createNode(t, ctx, repo, "n1", domain.NodeKindChat, "p1", 0)
	nodecmds.WaitQuiescentForTest(repo)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.ListByParent(ctx, "p1")
	require.Error(t, err, "a read-model failure must not read back as an empty node list")
}
