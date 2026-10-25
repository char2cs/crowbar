//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// recorded is a payload the claude CLI really sent, as its fixture keeps it,
// on the session c's CLI was launched with.
func (c turnChat) recorded(t *testing.T, name string) map[string]any {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "internal", "engine", "agents", "internal", "protocol",
		"testdata", "fixtures", "claude", name))
	require.NoError(t, err)
	var payload map[string]any
	require.NoError(t, json.Unmarshal(raw, &payload))
	payload["session_id"] = argAfter(c.launch.Argv, "--session-id")
	return payload
}

// A call into an MCP server names the server and its tool there.
func TestRegression_AnMCPToolRowNamesItsServerAndTool(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")

	c.hook(t, c.launch, "tool_pre", c.recorded(t, "PreToolUse.mcp.json"))

	var activity struct {
		ToolCalls []struct{ Kind, MCPServer, MCPTool string } `json:"toolCalls"`
	}
	c.h.get(repoBase(c.imported)+"/chats/"+c.chatID+"/activity", &activity)
	require.Len(t, activity.ToolCalls, 1)
	assert.Equal(t, "mcp", activity.ToolCalls[0].Kind)
	assert.Equal(t, "echo", activity.ToolCalls[0].MCPServer)
	assert.Equal(t, "shout", activity.ToolCalls[0].MCPTool)
}

// A permission for a call into an MCP server names the server it asks about.
func TestRegression_AChoiceForAnMCPCallNamesItsServer(t *testing.T) {
	c := newTurnChat(t, newHarness(t))
	c.startTurn(t, c.launch, "A")
	c.hook(t, c.launch, "tool_pre", c.recorded(t, "PreToolUse.mcp.json"))
	asked, err := json.Marshal(c.recorded(t, "PermissionRequest.mcp.json"))
	require.NoError(t, err)

	held := make(chan int, 1)
	go func() { held <- hookStatus(c.launch, c.h.url+repoBase(c.imported), "permission", string(asked)) }()
	frame := readUntil(t, c.conn, func(m map[string]any) bool {
		choices, _ := m["choices"].([]any)
		return c.frameFor("choice")(m) && len(choices) == 1
	})

	choice, _ := frame["choices"].([]any)[0].(map[string]any)
	assert.Equal(t, "echo", choice["mcpServer"])
	choiceID, _ := choice["id"].(string)
	c.h.post(repoBase(c.imported)+"/chats/"+c.chatID+"/choices/"+choiceID+"/answer",
		map[string]any{"optionIds": []string{"allow"}}, http.StatusOK, nil)
	assert.Equal(t, http.StatusOK, <-held, "the CLI is handed the answer")
}
