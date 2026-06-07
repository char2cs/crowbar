package v0

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/app/hub"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	lspdomain "github.com/char2cs/crowbar/api/internal/domain/lsp"
)

func TestWorkspacesDef_Lambdas(t *testing.T) {
	def := workspacesDef(nil)
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
	def := chatsDef(nil)
	evt := hub.ChatStatusEvent{ChatID: "c1", WsID: "w1", Status: domain.ChatStatusIdle}

	assert.Equal(t, "c1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "c1")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}

func TestGitDef_Lambdas(t *testing.T) {
	def := gitDef(nil)
	evt := gitdomain.GitStatusEvent{
		WsID:   "w1",
		Status: gitdomain.GitStatus{Branch: "main"},
	}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "main")
	assert.NotContains(t, string(data), "wsId")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}

func TestFilesDef_Lambdas(t *testing.T) {
	def := filesDef()
	evt := domain.FileChangeEvent{WsID: "w1", Path: "a.go"}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "a.go")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}

func TestLSPDef_Lambdas(t *testing.T) {
	def := lspDef(nil, nil)
	evt := lspdomain.DiagnosticsEvent{WsID: "w1"}

	assert.Equal(t, "w1", def.Namespace(evt))

	data, err := def.Serialize(evt)
	require.NoError(t, err)
	assert.Contains(t, string(data), "w1")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "w1", def.Filters[0].Extract(evt))
}

func TestChatStreamDef_Lambdas(t *testing.T) {
	def := chatStreamDef()
	frame := ChatFrame{ChatID: "c1"}

	assert.Equal(t, "c1", def.Namespace(frame))

	data, err := def.Serialize(frame)
	require.NoError(t, err)
	assert.Contains(t, string(data), "c1")

	require.Len(t, def.Filters, 1)
	assert.Equal(t, "c1", def.Filters[0].Extract(frame))
}
