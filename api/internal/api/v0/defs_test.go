package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func TestWorkspacesDef_Lambdas(t *testing.T) {
	def := workspacesDef()
	ws := domain.Workspace{ID: "w1", ProjectID: "p1", RepoID: "r1"}

	assert.Equal(t, "w1", def.Namespace(ws))

	data, err := def.Serialize(ws)
	require.NoError(t, err)
	assert.Contains(t, string(data), "w1")

	require.Len(t, def.Filters, 2)
	assert.Equal(t, "p1", def.Filters[0].Extract(ws))
	assert.Equal(t, "r1", def.Filters[1].Extract(ws))
}

func TestChatsDef_Lambdas(t *testing.T) {
	def := chatsDef()
	evt := hub.ChatStatusEvent{ChatID: "c1", WsID: "w1", Status: domain.ChatStatusIdle}

	assert.Equal(t, "c1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "c1")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}
