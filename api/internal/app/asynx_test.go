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

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
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

func (createNode) AggregateID() string                { return "n1" }
func (createNode) EventName() string                  { return "node.created.n1" }
func (createNode) ShouldSnapshot() bool               { return false }
func (createNode) Validate(*domain.Node) error        { return nil }
func (createNode) EmitEvent(*domain.Node) domain.Node { return domain.Node{ID: "n1"} }

// The attempt that parks an aggregate says so rather than promising a retry.
func TestNewAsynx_TheLastFailedAttemptIsLoggedAsParked(t *testing.T) {
	msgs := make(chan string, 64)
	prev := slog.Default()
	slog.SetDefault(slog.New(msgHandler{msgs: msgs}))
	t.Cleanup(func() { slog.SetDefault(prev) })
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	ax, err := newAsynx(es, asynxstore.NewSnapshots(), func(b *asynx.Builder[domain.Node]) {
		b.WithDeliveryMaxAttempts(1).
			WithSubscriber("node.*", func(context.Context, asynxModels.Event[domain.Node]) error {
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
