package v0

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// An operation goes out as the row the daemon recorded, nothing added.
func TestEncodeEntity_AnOperationIsItsRecordedRow(t *testing.T) {
	op := domain.Operation{ID: "op1", Kind: "git.push", State: domain.OpFailed, Version: 3}
	want, err := json.Marshal(op)
	require.NoError(t, err)

	got, err := EncodeEntity(stream.KindOperation, op)

	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
}

func TestEncodeEntity_RefusesAnUnknownKindOrAMismatchedValue(t *testing.T) {
	_, err := EncodeEntity("nope", domain.Operation{})
	assert.Equal(t, problem.Internal, problem.CodeOf(err))
	require.Error(t, err)

	_, err = EncodeEntity(stream.KindOperation, "not an operation")
	require.Error(t, err)
}
