package dto_test

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// TestRegression_AgentMessageDTO_CarriesTheReportedEffort.
//
// chatlog.Turn has carried Effort since claude's turn_stop mapping began reading
// effort.level, and the projection stores it in the `effort` column — but the
// wire shape had no field, so the value was written, read back and silently
// dropped at the API boundary. Dead data, invisible to every client.
//
// The fact is the provider's own report of what it USED, which is not the chat's
// requested selection: a chat that selected nothing can still receive one.
func TestRegression_AgentMessageDTO_CarriesTheReportedEffort(t *testing.T) {
	raw, err := json.Marshal(dto.AgentMessageDTO{Sequence: 1, TurnID: "t1", Effort: "high"})

	require.NoError(t, err)
	assert.Contains(t, string(raw), `"effort":"high"`)

	bare, err := json.Marshal(dto.AgentMessageDTO{Sequence: 1, TurnID: "t1"})
	require.NoError(t, err)
	assert.NotContains(t, string(bare), `"effort"`,
		"a provider that reported no effort must produce no field, never an empty one")
}

func TestAgentToolCallDTOFrom_CarriesWhatTheCallSaid(t *testing.T) {
	at := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	call := dto.AgentToolCallDTOFrom(domain.ActivityToolCall{
		ID: "t1", TurnID: "turn-1", Seq: 3, Name: "shout", Kind: "mcp", Target: "hi", Title: "Shout it",
		MCPServer: "echo", MCPTool: "shout", ExitCode: new(0), RequestRef: "sha256:in",
		Locations: []domain.ActivityToolLocation{{Path: "a.go", Line: 2}}, Status: domain.ToolStatusOK, StartedAt: at,
	})

	assert.Equal(t, "Shout it", call.Title)
	assert.Equal(t, "echo", call.MCPServer)
	assert.Equal(t, "shout", call.MCPTool)
	require.NotNil(t, call.ExitCode)
	assert.Equal(t, 0, *call.ExitCode, "a zero exit is an answer")
	assert.Equal(t, []dto.AgentToolLocationDTO{{Path: "a.go", Line: 2}}, call.Locations)
	assert.True(t, call.HasRequest)
	assert.False(t, call.HasResult)

	bare, err := json.Marshal(dto.AgentToolCallDTOFrom(domain.ActivityToolCall{ID: "t2", Name: "x", StartedAt: at}))
	require.NoError(t, err)
	for _, field := range []string{`"title"`, `"mcpServer"`, `"mcpTool"`, `"exitCode"`} {
		assert.NotContains(t, string(bare), field, "a call that says nothing of it carries no field")
	}
}
