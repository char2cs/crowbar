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
	agents "github.com/char2cs/crowbar/api/internal/engine/agents"
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

// The sidebar's kinds go out as the DTOs their list routes answer with.
func TestEncodeEntity_TheSidebarKindsAreTheirListDTOs(t *testing.T) {
	cases := map[stream.EntityKind]any{
		stream.KindProject:    dto.ProjectDTO{ID: "p1", Name: "P"},
		stream.KindRepo:       dto.RepoDTO{ID: "r1", ProjectID: "p1", Order: 2},
		stream.KindWorkspace:  dto.WorkspaceDTO{ID: "w1", RepoID: "r1", Branch: "main"},
		stream.KindChatEntity: dto.ChatEntityFrom(dto.AgentChatDTO{ID: "c1", Title: "t"}, "p1", "r1", true),
		stream.KindFolder:     dto.FolderEntityDTO{ID: "f1", RepoID: "r1", Name: "Docs"},
	}
	for kind, v := range cases {
		want, err := json.Marshal(v)
		require.NoError(t, err)
		got, err := EncodeEntity(kind, v)
		require.NoError(t, err, kind)
		assert.JSONEq(t, string(want), string(got), kind)
		_, err = EncodeEntity(kind, "not its DTO")
		assert.Error(t, err, kind)
	}
}

// A usage report goes out as the GET answered it.
func TestEncodeEntity_TelemetryIsItsReportDTO(t *testing.T) {
	used := 9
	report := agents.Telemetry{Source: "statusline", Context: &agents.ContextUsage{UsedTokens: &used}}
	want, err := json.Marshal(dto.AgentTelemetryDTOFrom(report))
	require.NoError(t, err)

	got, err := EncodeEntity(stream.KindTelemetry, report)

	require.NoError(t, err)
	assert.JSONEq(t, string(want), string(got))
}
