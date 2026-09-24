//go:build integration

package tests

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// baseEra writes aggregates into a crashed install's stores exactly as the
// pre-audit daemon (70ec430) left them on disk: one asynx event per aggregate,
// whose JSON patch carries the row in the shape that version marshalled, plus
// the read-model row it projected. Nothing of the current code's write path
// (validation, defaults, fields added since) touches it, so the next boot meets
// the same bytes an upgrading user's home holds.
//
// It is opened between a harness crash and the next boot; the stores are closed
// again before the new daemon opens them.
type baseEra struct {
	t        *testing.T
	adapters *adapter.Container
}

func openBaseEra(
	t *testing.T,
	home string,
) *baseEra {
	t.Helper()
	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	return &baseEra{t: t, adapters: adapters}
}

func (b *baseEra) close() {
	require.NoError(b.t, b.adapters.Close())
}

// workspace writes ws as base did. Base had no Provisioning or CreatedBranch
// field; both are omitempty, so a zero value marshals to base's bytes.
func (b *baseEra) workspace(
	ws domain.Workspace,
) {
	b.t.Helper()
	require.Empty(b.t, ws.Provisioning, "base never wrote provisioning")
	data := b.appendEvent(b.adapters.WorkspaceES(), "workspace.created."+ws.ID, ws.ID, 2, ws)
	require.NoError(b.t, b.adapters.WorkspaceView().Exec(
		"INSERT OR REPLACE INTO read_workspaces (id, data) VALUES (?, ?)", ws.ID, data).Error)
}

// tombstone records the delete of an existing workspace as the delete command
// writes it — the event a crash can leave with the purge still to run.
func (b *baseEra) tombstone(
	id string,
) {
	b.t.Helper()
	var data []byte
	require.NoError(b.t, b.adapters.WorkspaceView().
		Raw("SELECT data FROM read_workspaces WHERE id = ?", id).Row().Scan(&data))
	var ws domain.Workspace
	require.NoError(b.t, json.Unmarshal(data, &ws))
	ws.Status = domain.WorkspaceStatusDeleted
	ops := []map[string]any{{"op": "add", "path": "/status", "value": ws.Status}}
	b.appendPatch(b.adapters.WorkspaceES(), "workspace.deleted."+id, id, 2, ops)
	data, err := json.Marshal(ws)
	require.NoError(b.t, err)
	require.NoError(b.t, b.adapters.WorkspaceView().Exec(
		"UPDATE read_workspaces SET data = ? WHERE id = ?", data, id).Error)
}

// appendEvent records v as the aggregate's next event: an RFC 6902 patch from
// the zero state to v, which is what asynx's writer produces for a create.
func (b *baseEra) appendEvent(
	es eventLog,
	eventName string,
	id string,
	schemaVersion int,
	v any,
) []byte {
	b.t.Helper()
	data, err := json.Marshal(v)
	require.NoError(b.t, err)
	var fields map[string]json.RawMessage
	require.NoError(b.t, json.Unmarshal(data, &fields))
	ops := make([]map[string]any, 0, len(fields))
	for k, val := range fields {
		ops = append(ops, map[string]any{"op": "add", "path": "/" + k, "value": val})
	}
	b.appendPatch(es, eventName, id, schemaVersion, ops)
	return data
}

// appendPatch records ops as the aggregate's next event.
func (b *baseEra) appendPatch(
	es eventLog,
	eventName string,
	id string,
	schemaVersion int,
	ops []map[string]any,
) {
	b.t.Helper()
	ctx := context.Background()
	patches, err := json.Marshal(ops)
	require.NoError(b.t, err)
	prior, err := es.ReadFrom(ctx, "events:"+id, 1)
	require.NoError(b.t, err)
	version := int64(len(prior) + 1)
	evt, err := json.Marshal(map[string]any{
		"id":             uuid.NewString(),
		"event_name":     eventName,
		"version":        version,
		"schema_version": schemaVersion,
		"occurred_at":    time.Now().UTC(),
		"patches":        json.RawMessage(patches),
	})
	require.NoError(b.t, err)
	require.NoError(b.t, es.Append(ctx, "events:"+id, version, evt))
}

// eventLog is the part of an asynx event store the seed writes through.
type eventLog interface {
	Append(ctx context.Context, aggregateID string, version int64, data []byte) error
	ReadFrom(ctx context.Context, aggregateID string, fromVersion int64) ([][]byte, error)
}
