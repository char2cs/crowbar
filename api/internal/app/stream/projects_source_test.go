package stream_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
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
	if kind == "" {
		return nil, errors.New("unknown kind")
	}
	return json.Marshal(v)
}

// sidebar is a fake sidebar read; with gate set it waits there, after telling entered.
type sidebar struct {
	entities []appstream.Entity
	unread   []string
	err      error
	entered  chan struct{}
	gate     chan struct{}
	reads    atomic.Int32
}

func (s *sidebar) Entities(context.Context) (appstream.Snapshot, error) {
	s.reads.Add(1)
	if s.gate != nil {
		s.entered <- struct{}{}
		<-s.gate
	}
	return appstream.Snapshot{Entities: s.entities, Unread: s.unread}, s.err
}

func TestProjectsSource_SnapshotsEveryKindThenSynced(t *testing.T) {
	ops := []domain.Operation{{ID: "op1", State: domain.OpRunning}}
	side := &sidebar{entities: []appstream.Entity{
		{Kind: stream.KindProject, ID: "p1", Data: "project"},
		{Kind: stream.KindRepo, ID: "r1", Data: "repo"},
		{Kind: stream.KindWorkspace, ID: "w1", Data: "workspace"},
		{Kind: stream.KindChatEntity, ID: "c1", Data: "chat"},
		{Kind: stream.KindFolder, ID: "f1", Data: "folder"},
	}}
	src := appstream.NewProjectsSource(fixedOperations{ops: ops}, side, encodeOperation)
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: src}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "projects")
	frames := conn.until(t, syncedOn("projects"))

	var kinds []string
	for _, f := range frames[:len(frames)-1] {
		assert.Equal(t, "projects", f.Scope)
		kinds = append(kinds, f.Kind+":"+f.ID)
	}
	assert.Equal(t, []string{"operation:op1", "project:p1", "repo:r1", "workspace:w1", "chat:c1", "folder:f1"}, kinds)
	assert.Equal(t, `"chat"`, string(frames[4].Data))
}

// A broken read is transient like any failed snapshot.
func TestProjectsSource_AFailedReadClosesTheConnectionWithResync(t *testing.T) {
	src := appstream.NewProjectsSource(fixedOperations{}, &sidebar{err: problem.ErrUnavailable}, encodeOperation)
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: src}, nil)
	conn := newFakeConn()
	done := served(t, hub, conn)

	conn.send(t, stream.OpSub, "projects")

	ended(t, done)
	assert.Equal(t, stream.CloseResync, conn.closeCode())
}

func TestProjectsSource_SnapshotsEveryOperationInOrder(t *testing.T) {
	ops := []domain.Operation{{ID: "op1", State: domain.OpFailed}, {ID: "op2", State: domain.OpRunning}}
	src := appstream.NewProjectsSource(fixedOperations{ops: ops}, &sidebar{}, encodeOperation)
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

// A failing source is transient: the client reconnects and asks again.
func TestProjectsSource_AFailedSnapshotClosesTheConnectionWithResync(t *testing.T) {
	src := appstream.NewProjectsSource(fixedOperations{err: problem.ErrUnavailable}, &sidebar{}, encodeOperation)
	hub := newHub(map[stream.ScopeKind]appstream.Source{stream.KindProjects: src}, nil)
	conn := newFakeConn()
	done := served(t, hub, conn)

	conn.send(t, stream.OpSub, "projects")

	ended(t, done)
	assert.Equal(t, stream.CloseResync, conn.closeCode())
}

// The parts a read could not take ride its synced, so the client keeps them.
func TestProjectsSource_TheUnreadPartsRideTheSynced(t *testing.T) {
	side := &sidebar{entities: []appstream.Entity{{Kind: stream.KindRepo, ID: "r1", Data: "repo"}}, unread: []string{"r2"}}
	hub := newHub(map[stream.ScopeKind]appstream.Source{
		stream.KindProjects: appstream.NewProjectsSource(fixedOperations{}, side, encodeOperation),
	}, nil)
	conn := newFakeConn()
	served(t, hub, conn)

	conn.send(t, stream.OpSub, "projects")
	frames := conn.until(t, syncedOn("projects"))

	assert.Equal(t, []string{"r2"}, frames[len(frames)-1].Unread)
}
