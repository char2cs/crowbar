package chat_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"testing/synctest"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/testutil"
)

type captureBroadcast struct {
	mu   sync.Mutex
	rows []string
}

func (c *captureBroadcast) watch(e chat.ChatEvent) {
	c.push(e.ChatID, e.WorkspaceID)
}

func (c *captureBroadcast) push(chatID, workspaceID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rows = append(c.rows, chatID+":"+workspaceID)
}

func (c *captureBroadcast) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.rows)
}

func newRepoWithDeps(
	t *testing.T,
) (context.Context, chat.EventStore, *gormdb.DB, *captureBroadcast) {
	t.Helper()
	return newRepoOver(t, testutil.Placements(t))
}

func newRepo(
	t *testing.T,
) (context.Context, chat.EventStore) {
	t.Helper()
	ctx, repo, _, _ := newRepoWithDeps(t)
	return ctx, repo
}

func createChat(
	t *testing.T,
	ctx context.Context,
	repo chat.EventStore,
	id string,
	wsID string,
	now time.Time,
) domain.Chat {
	t.Helper()
	chat, err := repo.Create(ctx, chat.CreateInput{ID: id, WorkspaceID: wsID, Type: domain.ChatTypeChat, Now: now})
	require.NoError(t, err)
	return chat
}

func TestAgentChat_CreateAndGetChat(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	created := createChat(t, ctx, repo, "c1", "w1", now)
	assert.Equal(t, "c1", created.ID)
	assert.Equal(t, now, created.CreatedAt)

	got, err := repo.GetChat(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "w1", got.WorkspaceID)
}

func TestAgentChat_Create_ADuplicateIsAnIDConflict(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)

	_, err := repo.Create(ctx, chat.CreateInput{ID: "c1", WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: now})
	assert.Equal(t, problem.IDConflict, problem.CodeOf(err))
}

func TestAgentChat_GetChat_MissingBridgesToPackageErrNotFound(t *testing.T) {
	ctx, repo := newRepo(t)
	_, err := repo.GetChat(ctx, "does-not-exist")
	require.Error(t, err)
	assert.ErrorIs(t, err, chat.ErrNotFound)
}

func TestAgentChat_SetTitle_UserSourceLocksAgainstAgent(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)

	derived, err := repo.SetTitle(ctx, "c1", "Derived title", "derived")
	require.NoError(t, err)
	assert.Equal(t, "Derived title", derived.Title)
	assert.False(t, derived.TitleLocked)

	userTitled, err := repo.SetTitle(ctx, "c1", "User title", "user")
	require.NoError(t, err)
	assert.Equal(t, "User title", userTitled.Title)
	assert.True(t, userTitled.TitleLocked)

	_, err = repo.SetTitle(ctx, "c1", "Agent override", "agent")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestAgentChat_Forget_ErasesAggregate(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	createChat(t, ctx, repo, "c1", "w1", time.Unix(1, 0).UTC())
	chat.WaitQuiescentForTest(repo)

	require.NoError(t, repo.Forget(ctx, "c1"))
	chat.WaitQuiescentForTest(repo)

	var n int64
	require.NoError(t, db.WithContext(ctx).Table("agent_chats_read").Count(&n).Error)
	assert.Zero(t, n, "Forget's OnForget must drop the read-model row")

	_, err := repo.GetChat(ctx, "c1")
	require.Error(t, err)
	assert.ErrorIs(t, err, chat.ErrNotFound)
}

func TestAgentChat_ListByWorkspace_FiltersWsID(t *testing.T) {
	ctx, repo := newRepo(t)
	now := time.Unix(1, 0).UTC()
	createChat(t, ctx, repo, "c1", "w1", now)
	createChat(t, ctx, repo, "c2", "w2", now)
	chat.WaitQuiescentForTest(repo)

	list, err := repo.ListByWorkspace(ctx, "w1")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "c1", list[0].ID)

	none, err := repo.ListByWorkspace(ctx, "no-match")
	require.NoError(t, err)
	assert.Empty(t, none)
}

func TestAgentChat_HubBroadcastsChatIDAndKind(t *testing.T) {
	ctx, repo, _, cap := newRepoWithDeps(t)
	createChat(t, ctx, repo, "c1", "w1", time.Unix(1, 0).UTC())

	chat.WaitQuiescentForTest(repo)
	assert.GreaterOrEqual(t, cap.count(), 1, "hub projection must broadcast a lifecycle frame")
}

const concurrentCreateTrials = 10

func TestAgentChat_ConcurrentCreate_OneWins(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)
	now := time.Unix(1000, 0).UTC()

	for trial := range concurrentCreateTrials {
		chatID := fmt.Sprintf("race-chat-%d", trial)

		results := make([]error, 2)
		var g errgroup.Group
		for i := range results {
			g.Go(func() error {
				_, results[i] = repo.Create(ctx, chat.CreateInput{
					ID: chatID, WorkspaceID: "w1", Type: domain.ChatTypeChat, Now: now,
				})
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
			require.Equalf(t, problem.IDConflict, problem.CodeOf(e),
				"trial %d: a losing Create must be rejected by Validate, not left as an unresolved pipeline failure", trial)
		}
		require.Equalf(t, 1, succeeded, "trial %d: exactly one racing Create must commit; the other must be rejected", trial)

		chat.WaitQuiescentForTest(repo)
		got, err := repo.GetChat(ctx, chatID)
		require.NoError(t, err)
		require.Equalf(t, "w1", got.WorkspaceID, "trial %d: the read model must hold exactly one chat for the id", trial)
	}
}

func TestAgentChat_OpenReadModel_ErrorOnBadDB(t *testing.T) {
	db, err := storesqlite.OpenDB(":memory:")
	require.NoError(t, err)
	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = chat.OpenReadModel(db)
	require.Error(t, err)
}

func TestPlacement_IsCurrentWhileTheProjectionIsStillBehind(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)

	_, err := repo.Create(ctx, chat.CreateInput{ID: "c1", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	_, err = repo.Create(ctx, chat.CreateInput{ID: "c2", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	chat.WaitQuiescentForTest(repo)

	var unplaced []byte
	require.NoError(t,
		db.Raw("SELECT data FROM agent_chats_read WHERE id = ?", "c2").Row().Scan(&unplaced))

	_, err = repo.SetPlacement(ctx, "c2", "c1", 0)
	require.NoError(t, err)
	chat.WaitQuiescentForTest(repo)
	require.NoError(t,
		db.Exec("UPDATE agent_chats_read SET data = ? WHERE id = ?", unplaced, "c2").Error)

	projected, err := repo.GetChat(ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "c1", projected.ParentID, "the projected read")
	loaded, err := repo.LoadChat(ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "c1", loaded.ParentID, "the read a thread's inheritance decides on")
}

func TestSetOrder_RenumbersWithoutMoving(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)

	_, err := repo.Create(ctx, chat.CreateInput{ID: "c1", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	_, err = repo.Create(ctx, chat.CreateInput{ID: "c2", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	_, err = repo.SetPlacement(ctx, "c2", "c1", 0)
	require.NoError(t, err)

	renumbered, err := repo.SetOrder(ctx, "c2", 3)
	require.NoError(t, err)
	assert.Equal(t, "c1", renumbered.ParentID)
	assert.Equal(t, 3, renumbered.Order)

	loaded, err := repo.LoadChat(ctx, "c2")
	require.NoError(t, err)
	assert.Equal(t, "c1", loaded.ParentID, "the chat is still threaded off the one it was filed under")
	assert.Equal(t, 3, loaded.Order)
}

func TestSetOrder_RefusesAChatThatDoesNotExist(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)

	_, err := repo.SetOrder(ctx, "no-such-chat", 0)
	assert.Error(t, err)
}

func TestSetWorkspace_FillsTheSlotAndReflectsImmediatelyInTheReadModel(t *testing.T) {
	ctx, repo := newRepo(t)
	createChat(t, ctx, repo, "c1", "", time.Now())

	updated, err := repo.SetWorkspace(ctx, "c1", "ws-1")
	require.NoError(t, err)
	assert.Equal(t, "ws-1", updated.WorkspaceID)

	got, err := repo.GetChat(ctx, "c1")
	require.NoError(t, err)
	assert.Equal(t, "ws-1", got.WorkspaceID)
}

func TestSetWorkspace_RefusesAChatThatDoesNotExist(t *testing.T) {
	ctx, repo := newRepo(t)

	_, err := repo.SetWorkspace(ctx, "no-such-chat", "ws-1")
	assert.Error(t, err)
}

func TestLoadChat_UnknownChatIsTheSameNotFoundGetChatReports(t *testing.T) {
	ctx, repo, _, _ := newRepoWithDeps(t)

	_, err := repo.LoadChat(ctx, "no-such-chat")
	assert.ErrorIs(t, err, chat.ErrNotFound)
}

func TestGetChat_AReadFailureIsNotAMiss(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)

	_, err := repo.Create(ctx, chat.CreateInput{ID: "c1", WorkspaceID: "ws-1", Type: domain.ChatTypeChat, Now: time.Now()})
	require.NoError(t, err)
	chat.WaitQuiescentForTest(repo)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.GetChat(ctx, "c1")
	require.Error(t, err)
	assert.NotErrorIs(t, err, chat.ErrNotFound,
		"a chat that cannot be READ is not a chat that is gone")
}

func TestAgentChat_MutationsOnUnknownChat_SurfaceTheCommandsValidationError(t *testing.T) {
	ctx, repo := newRepo(t)

	t.Run("SetSelection", func(t *testing.T) {
		_, err := repo.SetSelection(ctx, "no-such-chat", "claude", "high")
		require.Error(t, err)
		assert.ErrorIs(t, err, asynxModels.ErrValidation)
	})

	t.Run("SetPermissionLevel", func(t *testing.T) {
		_, err := repo.SetPermissionLevel(ctx, "no-such-chat", "guarded")
		require.Error(t, err)
		assert.ErrorIs(t, err, asynxModels.ErrValidation)
	})

	t.Run("SetPlacement", func(t *testing.T) {
		_, err := repo.SetPlacement(ctx, "no-such-chat", "", 0)
		require.Error(t, err)
		assert.ErrorIs(t, err, asynxModels.ErrValidation)
	})
}

func TestAgentChat_Forget_RefusesAChatThatDoesNotExist(t *testing.T) {
	ctx, repo := newRepo(t)
	err := repo.Forget(ctx, "no-such-chat")
	require.Error(t, err)
	assert.ErrorIs(t, err, asynxModels.ErrValidation)
}

func TestAgentChat_ListReads_PropagateStorageFailure(t *testing.T) {
	ctx, repo, db, _ := newRepoWithDeps(t)
	createChat(t, ctx, repo, "c1", "w1", time.Unix(1, 0).UTC())
	chat.WaitQuiescentForTest(repo)

	sqlDB, err := db.DB()
	require.NoError(t, err)
	require.NoError(t, sqlDB.Close())

	_, err = repo.ListChats(ctx)
	require.Error(t, err, "a read-model failure must not read back as an empty chat list")

	_, err = repo.ListByWorkspace(ctx, "w1")
	require.Error(t, err, "a read-model failure must not read back as an empty chat list")
}

func TestAgentChat_SeedPermissionLevelReturnsOnceItIsPublished(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		es := asynxstore.New()
		db, err := storesqlite.OpenDB(":memory:")
		require.NoError(t, err)
		sqlDB, err := db.DB()
		require.NoError(t, err)
		t.Cleanup(func() { _ = sqlDB.Close() })
		rm, err := chat.OpenReadModel(db)
		require.NoError(t, err)
		b := asynx.New[domain.Chat]().WithEventStore(es).WithSnapshotStore(asynxstore.NewSnapshots())
		chat.Subscriber(rm)(b)
		ax, err := b.Build()
		require.NoError(t, err)
		t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
		release := make(chan struct{})
		free := sync.OnceFunc(func() { close(release) })
		t.Cleanup(free)
		watch := func(e chat.ChatEvent) {
			if e.Chat.PermissionLevel == "guarded" {
				<-release
			}
		}
		ctx := context.Background()
		repo, err := chat.NewEventSourced(ctx, ax, es, rm, watch, testutil.Placements(t))
		require.NoError(t, err)
		createChat(t, ctx, repo, "c1", "w1", time.Unix(1000, 0))

		returned := make(chan error, 1)
		go func() {
			_, err := repo.SeedPermissionLevel(ctx, "c1", "guarded")
			returned <- err
		}()
		synctest.Wait()
		select {
		case <-returned:
			t.Fatal("the seed returned while its chat was still being published")
		default:
		}
		free()
		require.NoError(t, <-returned)
	})
}
