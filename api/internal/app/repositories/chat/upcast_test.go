package chat_test

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/chat"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// v1Chat is the chat document as schema 1 wrote it, placement and the turn
// state the ledger now folds included.
type v1Chat struct {
	domain.Chat
	ParentID           string     `json:"parentId,omitempty"`
	Order              int        `json:"order"`
	Working            bool       `json:"working"`
	CurrentTurnStarted *time.Time `json:"currentTurnStarted,omitempty"`
	LastActivityAt     time.Time  `json:"lastActivityAt"`
	AsyncWork          int        `json:"asyncWork"`
	LedgerCursor       int        `json:"ledgerCursor"`
}

// v1Edit is any schema-1 write: the next document from the current one.
type v1Edit struct {
	id   string
	name string
	next func(v1Chat) v1Chat
}

func (e v1Edit) EmitEvent(cur *v1Chat) v1Chat {
	if cur == nil {
		return e.next(v1Chat{})
	}
	return e.next(*cur)
}

func (e v1Edit) AggregateID() string    { return e.id }
func (e v1Edit) EventName() string      { return e.name }
func (e v1Edit) ShouldSnapshot() bool   { return false }
func (e v1Edit) Validate(*v1Chat) error { return nil }

// writeV1History appends, at schema 1, a chat's creation, a move, a renumber
// and a turn: history that patches /parentId, /order and the turn state.
func writeV1History(t *testing.T, es asynxModels.Store, id string) {
	t.Helper()
	ctx := context.Background()
	ax, err := asynx.New[v1Chat]().WithEventStore(es).WithSnapshotStore(asynxstore.NewSnapshots()).
		WithSchemaVersion(1).Build()
	require.NoError(t, err)
	defer func() { require.NoError(t, ax.Shutdown(ctx)) }()
	writes := []v1Edit{
		{id, "created", func(v1Chat) v1Chat {
			return v1Chat{Chat: domain.Chat{ID: id, Title: "Notes", WorkspaceID: "ws-1", Type: domain.ChatTypeChat}}
		}},
		{id, "placement_set", func(c v1Chat) v1Chat { c.ParentID, c.Order = "f-1", 3; return c }},
		{id, "order_set", func(c v1Chat) v1Chat { c.Order = 1; return c }},
		{id, "turn_started", func(c v1Chat) v1Chat {
			at := time.Unix(200, 0).UTC()
			c.Working, c.CurrentTurnStarted, c.LastActivityAt, c.AsyncWork, c.LedgerCursor = true, &at, at, 1, 4
			return c
		}},
		{id, "turn_stopped", func(c v1Chat) v1Chat {
			c.Working, c.CurrentTurnStarted, c.LastActivityAt, c.AsyncWork = false, nil, time.Unix(300, 0).UTC(), 0
			return c
		}},
		{id, "titled", func(c v1Chat) v1Chat { c.Title = "Notes, renamed"; return c }},
	}
	for _, w := range writes {
		_, err := ax.SendWait(ctx, w)
		require.NoError(t, err)
	}
}

func TestRetiredFields_DropsPlacementAndTurnStatePatches(t *testing.T) {
	ctx := context.Background()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	writeV1History(t, es, "c-1")

	b := asynx.New[domain.Chat]().WithEventStore(es).WithSnapshotStore(asynxstore.NewSnapshots())
	chat.RetiredFields(b)
	ax, err := b.Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ax.Shutdown(ctx)) })

	got, err := ax.Get(ctx, "c-1")
	require.NoError(t, err)
	assert.Equal(t, "Notes, renamed", got.Title, "every other patch still applies")
	assert.Equal(t, "ws-1", got.WorkspaceID)
	assert.Empty(t, got.ParentID, "placement is the Tree's to answer")
	assert.Zero(t, got.Order)
}

// Without the upcaster the same history does not load: a patch replaces a
// field the chat document no longer has.
func TestRetiredFields_IsWhatLetsSchema1HistoryLoad(t *testing.T) {
	ctx := context.Background()
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	writeV1History(t, es, "c-1")

	ax, err := asynx.New[domain.Chat]().WithEventStore(es).WithSnapshotStore(asynxstore.NewSnapshots()).Build()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, ax.Shutdown(ctx)) })

	_, err = ax.Get(ctx, "c-1")
	require.Error(t, err)
}
