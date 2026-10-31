package stream_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type fixedOperations struct {
	ops []domain.Operation
	err error
}

func (f fixedOperations) Snapshot(context.Context) ([]domain.Operation, error) { return f.ops, f.err }

func encodeOperation(kind stream.EntityKind, v any) (json.RawMessage, error) {
	if kind != stream.KindOperation {
		return nil, errors.New("unknown kind")
	}
	return json.Marshal(v)
}

func TestProjectsSource_SnapshotsEveryOperationInOrder(t *testing.T) {
	ops := []domain.Operation{{ID: "op1", State: domain.OpFailed}, {ID: "op2", State: domain.OpRunning}}
	src := appstream.NewProjectsSource(fixedOperations{ops: ops}, encodeOperation)
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: src}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	snapshot := conn.sub(t, "projects")

	require.Len(t, snapshot, 2)
	for i, f := range snapshot {
		assert.Equal(t, string(stream.KindOperation), f.Kind)
		assert.Equal(t, ops[i].ID, f.ID)
		var got domain.Operation
		require.NoError(t, json.Unmarshal(f.Data, &got))
		assert.Equal(t, ops[i], got)
	}
}

func TestProjectsSource_AFailedSnapshotRefusesTheScope(t *testing.T) {
	src := appstream.NewProjectsSource(fixedOperations{err: problem.ErrUnavailable}, encodeOperation)
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: src}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "projects")
	closed := conn.next(t)

	assert.Equal(t, stream.FrameClosed, closed.T)
	assert.Equal(t, stream.ReasonRefused, closed.Reason)
	require.NotNil(t, closed.Problem)
	assert.Equal(t, problem.Unavailable, closed.Problem.Code)
}
