package node_test

import (
	"context"
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
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	rm, err := nodecmds.OpenReadModel(db)
	require.NoError(t, err)
	b := asynx.New[domain.Node]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	nodecmds.Subscriber(rm)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	cap := &captureBroadcast{}
	repo, err := nodecmds.NewEventSourced(ax, es, rm, cap.watch)
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
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	rm, err := nodecmds.OpenReadModel(db)
	require.NoError(t, err)
	b := asynx.New[domain.Node]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	nodecmds.Subscriber(rm)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	repo, err := nodecmds.NewEventSourced(ax, es, rm, nil)
	require.NoError(t, err)

	assert.NotPanics(t, func() {
		_, err := repo.Create(context.Background(), "n1", domain.NodeKindChat, "", 0)
		require.NoError(t, err)
	})
}

// concurrentCreateTrials is the number of independent trials
// TestNode_ConcurrentCreate_OneWins runs, mirroring agentchat's own race test.
const concurrentCreateTrials = 10

// Two Creates of the SAME id race: exactly one commits and the loser, run after
// it, is refused by Validate (asynxModels.ErrValidation), never ErrPipelineFailed.
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

func TestNode_OpenReadModel_ErrorOnBadDB(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = nodecmds.OpenReadModel(db)
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

// nodePlacementLog records, per placement announcement, the parent the READ
// MODEL held for the node at that moment ("gone" once deleted).
type nodePlacementLog struct {
	mu   sync.Mutex
	repo nodecmds.EventStore
	seen []string
}

func (p *nodePlacementLog) record(id string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	all, err := p.repo.ListAll(context.Background())
	if err != nil {
		p.seen = append(p.seen, id+":error")
		return
	}
	for _, n := range all {
		if n.ID == id {
			p.seen = append(p.seen, id+":"+n.ParentID)
			return
		}
	}
	p.seen = append(p.seen, id+":gone")
}

func (p *nodePlacementLog) entries() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.seen...)
}

// TestNode_PlacementWatchFiresAfterTheReadModelSaved: a re-parent or delete is
// announced only once the read model holds it; a pure renumber is not.
func TestNode_PlacementWatchFiresAfterTheReadModelSaved(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	log := &nodePlacementLog{}
	rm, err := nodecmds.OpenReadModel(db, nodecmds.WithPlacementWatch(log.record))
	require.NoError(t, err)
	b := asynx.New[domain.Node]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	nodecmds.Subscriber(rm)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	repo, err := nodecmds.NewEventSourced(ax, es, rm, nil)
	require.NoError(t, err)
	log.repo = repo
	ctx := context.Background()

	createNode(t, ctx, repo, "keep", domain.NodeKindChat, "", 0)
	createNode(t, ctx, repo, "f1", domain.NodeKindFolder, "owner-a", 0)
	require.NoError(t, repo.SetOrder(ctx, "f1", 3))
	require.NoError(t, repo.SetPlacement(ctx, "f1", "owner-b", 0))
	require.NoError(t, repo.Forget(ctx, "f1"))
	ax.WaitPublish()

	assert.Equal(t, []string{"keep:", "f1:owner-a", "f1:owner-b", "f1:gone"}, log.entries())
}
