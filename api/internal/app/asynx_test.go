package app

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

func TestNewAsynx_BuildsInstance(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ss, err := eventsqlite.NewSnapshotStore(":memory:")
	require.NoError(t, err)
	ax, err := newAsynx[domain.Workspace](es, ss)
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })
	assert.NotNil(t, ax)
}

func TestNewAsynx_NilStore_ReturnsError(t *testing.T) {
	_, err := newAsynx[domain.Workspace](nil, nil)
	assert.Error(t, err)
}

func TestNewAsynx_NilSnapshotStore_ReturnsError(t *testing.T) {
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	_, err = newAsynx[domain.Workspace](es, nil)
	assert.Error(t, err)
}

// msgHandler sends each log message to msgs, dropping it once nobody reads.
type msgHandler struct{ msgs chan<- string }

func (h msgHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h msgHandler) Handle(_ context.Context, r slog.Record) error {
	select {
	case h.msgs <- r.Message:
	default:
	}
	return nil
}
func (h msgHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h msgHandler) WithGroup(string) slog.Handler      { return h }

type createNode struct{}

func (createNode) AggregateID() string            { return "n1" }
func (createNode) EventName() string              { return "node.created.n1" }
func (createNode) ShouldSnapshot() bool           { return false }
func (createNode) Validate(*tree.Node) error      { return nil }
func (createNode) EmitEvent(*tree.Node) tree.Node { return tree.Node{ID: "n1"} }

// The attempt that parks an aggregate says so rather than promising a retry.
func TestNewAsynx_TheLastFailedAttemptIsLoggedAsParked(t *testing.T) {
	msgs := make(chan string, 64)
	prev := slog.Default()
	slog.SetDefault(slog.New(msgHandler{msgs: msgs}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := newAsynx(es, asynxstore.NewSnapshots(), func(b *asynx.Builder[tree.Node]) {
		b.WithDeliveryMaxAttempts(1).
			WithSubscriber("node.*", func(context.Context, asynxModels.Event[tree.Node]) error {
				return errors.New("read model down")
			})
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	_, err = ax.Send(context.Background(), createNode{})
	require.NoError(t, err)
	deadline := time.After(10 * time.Second)
	for {
		select {
		case msg := <-msgs:
			if strings.Contains(msg, "parked") {
				assert.NotContains(t, msg, "retrying")
				return
			}
		case <-deadline:
			t.Fatal("the attempt that parked the aggregate was never logged as parked")
		}
	}
}

// v1Chat is the chat document as schema 1 wrote it, placement included.
type v1Chat struct {
	domain.Chat
	ParentID string `json:"parentId,omitempty"`
	Order    int    `json:"order"`
}

type v1Write struct{ doc v1Chat }

func (w v1Write) AggregateID() string      { return w.doc.ID }
func (w v1Write) EventName() string        { return "agent_chat.written." + w.doc.ID }
func (w v1Write) ShouldSnapshot() bool     { return false }
func (w v1Write) Validate(*v1Chat) error   { return nil }
func (w v1Write) EmitEvent(*v1Chat) v1Chat { return w.doc }

// A home the retired build wrote, whose chat history patches /parentId and
// /order, still loads through the daemon's own chat store.
func TestNewAggregates_AChatHistoryThatPatchedPlacementLoads(t *testing.T) {
	ctx := context.Background()
	adapters, err := adapter.New(adapter.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })
	old, err := asynx.New[v1Chat]().WithEventStore(adapters.AgentChatES()).
		WithSnapshotStore(asynxstore.NewSnapshots()).WithSchemaVersion(1).Build()
	require.NoError(t, err)
	born := v1Chat{Chat: domain.Chat{ID: "c-1", Title: "Notes", Type: domain.ChatTypeChat}}
	placed := born
	placed.ParentID, placed.Order = "f-1", 3
	for _, doc := range []v1Chat{born, placed} {
		_, err := old.SendWait(ctx, v1Write{doc: doc})
		require.NoError(t, err)
	}
	require.NoError(t, old.Shutdown(ctx))
	eng, err := engine.New(ctx)
	require.NoError(t, err)
	t.Cleanup(eng.Close)

	c, err := New(ctx, eng, adapters, migratetest.NoPostOpen())
	require.NoError(t, err)

	got, err := c.Repositories.AgentChat.LoadChat(ctx, "c-1")
	require.NoError(t, err)
	assert.Equal(t, "Notes", got.Title)
}
