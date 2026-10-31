package v0

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	appstream "github.com/char2cs/crowbar/api/internal/app/stream"
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

// A review thread goes out as its ThreadDTO, whether a REST write hands the DTO
// or the stream hands the thread and the ids of its scope.
func TestEncodeEntity_AReviewThreadIsItsThreadDTO(t *testing.T) {
	th := domain.ReviewThread{ID: "th1", WsID: "w1", FilePath: "a.go"}
	want, err := json.Marshal(dto.ThreadDTOFrom(th.NormalizedMessages(), "p1", "r1"))
	require.NoError(t, err)

	fromThread, err := EncodeEntity(stream.KindReviewThread, appstream.ReviewThread{Thread: th, ProjectID: "p1", RepoID: "r1"})
	require.NoError(t, err)
	fromDTO, err := EncodeEntity(stream.KindReviewThread, dto.ThreadDTOFrom(th.NormalizedMessages(), "p1", "r1"))
	require.NoError(t, err)

	assert.JSONEq(t, string(want), string(fromThread))
	assert.JSONEq(t, string(want), string(fromDTO))
	_, err = EncodeEntity(stream.KindReviewThread, th)
	assert.Error(t, err, "a bare thread names no project or repo")
}

func TestEncodeEntity_RefusesAnUnknownKindOrAMismatchedValue(t *testing.T) {
	_, err := EncodeEntity("nope", domain.Operation{})
	assert.Equal(t, problem.Internal, problem.CodeOf(err))
	require.Error(t, err)

	_, err = EncodeEntity(stream.KindOperation, "not an operation")
	require.Error(t, err)
}
