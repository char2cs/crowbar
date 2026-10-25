package dto_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestAgentChoiceDTOsFrom_SaysWhatTheCallActsOnAndWhichAreHeld(t *testing.T) {
	choices := dto.AgentChoiceDTOsFrom([]domain.ActivityChoice{
		{ID: "c1", Kind: domain.ChoiceKindPermission, ToolName: "shout", ToolTarget: "hi", MCPServer: "echo"},
		{ID: "c2", Kind: domain.ChoiceKindPermission},
	}, []string{"c1"})

	require.Len(t, choices, 2)
	assert.Equal(t, "hi", choices[0].ToolTarget)
	assert.Equal(t, "echo", choices[0].MCPServer)
	assert.True(t, choices[0].Answerable)
	assert.False(t, choices[1].Answerable)
	assert.Empty(t, choices[1].ToolTarget)
}
