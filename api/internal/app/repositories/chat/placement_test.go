package chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	gormdb "gorm.io/gorm"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

// newRepoOver is newRepoWithDeps over the placements given.
func newRepoOver(
	t *testing.T,
	placements chat.Placements,
) (context.Context, chat.EventStore, *gormdb.DB, *captureBroadcast) {
	t.Helper()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	rm, err := chat.OpenReadModel(db)
	require.NoError(t, err)
	b := asynx.New[domain.Chat]().
		WithEventStore(es).
		WithSnapshotStore(asynxstore.NewSnapshots()).
		WithShardingOpts(asynx.ShardingOpts{Shards: 8, QueueDepth: 1000})
	chat.Subscriber(rm)(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	cap := &captureBroadcast{}
	repo, err := chat.NewEventSourced(context.Background(), ax, es, rm, cap.watch, placements)
	require.NoError(t, err)
	return context.Background(), repo, db, cap
}

// countedPlacements counts the reads of one chat's placement and of all of them.
type countedPlacements struct {
	chat.Placements
	one, all int
}

func (c *countedPlacements) ChatPlacement(ctx context.Context, chatID string) (tree.Placement, error) {
	c.one++
	return c.Placements.ChatPlacement(ctx, chatID)
}

func (c *countedPlacements) ChatPlacements(ctx context.Context, chatIDs []string) (map[string]tree.Placement, error) {
	c.all++
	return c.Placements.ChatPlacements(ctx, chatIDs)
}

// GetChats places every chat it finds in one placement read, and skips an id
// no chat has.
func TestAgentChat_GetChats_PlacesEveryChatInOneRead(t *testing.T) {
	counted := &countedPlacements{Placements: testutil.Placements(t)}
	ctx, repo, _, _ := newRepoOver(t, counted)
	now := time.Unix(1, 0).UTC()
	for _, id := range []string{"c1", "c2", "c3"} {
		createChat(t, ctx, repo, id, "ws", now)
	}
	_, err := repo.SetPlacement(ctx, "c2", "f-1", 4)
	require.NoError(t, err)
	require.Eventually(t, func() bool { _, err := repo.GetChat(ctx, "c3"); return err == nil }, 5*time.Second, time.Millisecond)
	counted.one, counted.all = 0, 0

	got, err := repo.GetChats(ctx, []string{"c1", "nope", "c2", "c3"})

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.Equal(t, "f-1", got[1].ParentID)
	assert.Equal(t, 4, got[1].Order)
	assert.Equal(t, 1, counted.all)
	assert.Zero(t, counted.one)
}

// A chat GetChats cannot read fails the whole read rather than being skipped as
// a chat that is gone.
func TestAgentChat_GetChats_AReadFailureFailsTheRead(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	createChat(t, ctx, repo, "c1", "ws", time.Unix(1, 0).UTC())
	chat.WaitQuiescentForTest(repo)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	got, err := repo.GetChats(ctx, []string{"c1"})

	require.Error(t, err)
	require.NotErrorIs(t, err, chat.ErrNotFound)
	assert.Nil(t, got)
}
