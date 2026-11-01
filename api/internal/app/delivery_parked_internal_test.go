package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	asynxstore "github.com/char2cs/asynx/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eventsqlite "github.com/char2cs/crowbar/api/internal/adapter/eventstore/sqlite"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/tree"
)

// A read model that keeps refusing an event parks its aggregate; the user
// learns of it as a failed operation on the entity, not from the log.
func TestNewAsynx_AParkedAggregateIsAFailedOperationOnItsEntity(t *testing.T) {
	ops, frames := recordingOps(t)
	es, err := eventsqlite.NewEventStore(":memory:")
	require.NoError(t, err)
	parked := reportParked(ops, "sidebar", func(e asynxModels.Event[tree.Node]) (domain.EntityRef, string) {
		return domain.EntityRef{Kind: domain.EntityProject, ID: e.AggregateID}, e.AggregateID
	})
	ax, err := newAsynx(es, asynxstore.NewSnapshots(), parked, func(b *asynx.Builder[tree.Node]) {
		b.WithDeliveryMaxAttempts(1).
			WithSubscriber("node.*", func(context.Context, asynxModels.Event[tree.Node]) error {
				return errors.New("read model down")
			})
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = ax.Shutdown(context.Background()) })

	_, err = ax.Send(context.Background(), createNode{})
	require.NoError(t, err)

	var op domain.Operation
	select {
	case op = <-frames:
	case <-time.After(10 * time.Second):
		t.Fatal("the parked aggregate was never recorded")
	}
	assert.Equal(t, domain.OpEntitySave, op.Kind)
	assert.Equal(t, domain.EntityRef{Kind: domain.EntityProject, ID: "n1"}, op.Subject)
	assert.Equal(t, "n1", op.ProjectID)
	assert.Equal(t, "Save sidebar", op.Label)
	assert.Equal(t, domain.OpFailed, op.State)
	require.NotNil(t, op.Error)
	assert.Equal(t, problem.Unavailable, op.Error.Code)
	assert.Contains(t, op.Error.Message, "Crowbar stopped saving this sidebar's changes until it restarts")
	assert.Contains(t, op.Error.Message, "read model down")
}

func TestNewParkedReports_NameEachAggregatesEntity(t *testing.T) {
	ops, frames := recordingOps(t)
	reports := newParkedReports(ops)
	ctx := context.Background()
	cause := asynxModels.ErrDeliveryParked

	reports.workspace(ctx, asynxModels.Event[domain.Workspace]{AggregateID: "w1", Aggregate: domain.Workspace{ProjectID: "p1"}}, cause)
	reports.reviewThread(ctx, asynxModels.Event[domain.ReviewThread]{AggregateID: "t1", Aggregate: domain.ReviewThread{WsID: "w2"}}, cause)
	reports.agentChat(ctx, asynxModels.Event[domain.Chat]{AggregateID: "c1"}, cause)
	reports.tree(ctx, asynxModels.Event[tree.Tree]{AggregateID: "p3"}, cause)

	for _, want := range []struct {
		ref     domain.EntityRef
		project string
	}{
		{domain.EntityRef{Kind: domain.EntityWorkspace, ID: "w1"}, "p1"},
		{domain.EntityRef{Kind: domain.EntityWorkspace, ID: "w2"}, ""},
		{domain.EntityRef{Kind: domain.EntityChat, ID: "c1"}, ""},
		{domain.EntityRef{Kind: domain.EntityProject, ID: "p3"}, "p3"},
	} {
		op := published(t, frames)
		assert.Equal(t, want.ref, op.Subject)
		assert.Equal(t, want.project, op.ProjectID)
		assert.Equal(t, domain.OpFailed, op.State)
	}
}
