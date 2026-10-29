package v0

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type fixedOperations []domain.Operation

func (f fixedOperations) Snapshot(context.Context) ([]domain.Operation, error) { return f, nil }

// A client tells an operation it is shown on connect from one that just changed,
// so a failure it first meets live is announced and a snapshot's is not.
func TestOperationsDef_MarksSnapshotFramesOnly(t *testing.T) {
	def := operationsDef(fixedOperations{{ID: "op1", State: domain.OpFailed}})
	decode := func(frame operationFrame) map[string]any {
		raw, err := def.Serialize(frame)
		require.NoError(t, err)
		var got map[string]any
		require.NoError(t, json.Unmarshal(raw, &got))
		return got
	}

	snapshot := def.Snapshot("")
	require.Len(t, snapshot, 1)
	assert.Equal(t, true, decode(snapshot[0])["snapshot"])
	assert.Equal(t, "op1", decode(snapshot[0])["id"])

	live := decode(operationFrame{Operation: domain.Operation{ID: "op2"}})
	assert.NotContains(t, live, "snapshot")
	assert.Equal(t, "op2", live["id"])
}
