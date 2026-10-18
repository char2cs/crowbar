package dto_test

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
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
