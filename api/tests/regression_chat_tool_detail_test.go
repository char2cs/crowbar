//go:build integration

package tests

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
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

	calls := toolCalls(t, c.h, c.chatID)
	require.Len(t, calls, 1)
	assert.Equal(t, "mcp", calls[0].Kind)
	assert.Equal(t, "echo", calls[0].MCPServer)
	assert.Equal(t, "shout", calls[0].MCPTool)
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
	var asked1 []ledger.Ask
	require.Eventually(t, func() bool {
		asked1 = rowsOf(t, c.feedRows(t), ledger.KindAsk, func(b ledger.Ask) bool { return b.Kind != "" })
		return len(asked1) == 1
	}, 10*time.Second, 10*time.Millisecond, "the permission never opened an ask")

	assert.Equal(t, "echo", asked1[0].MCPServer)
	c.h.post(repoBase(c.imported)+"/chats/"+c.chatID+"/choices/"+asked1[0].AskID+"/answer",
		map[string]any{"optionIds": []string{"allow"}}, http.StatusOK, nil)
	assert.Equal(t, http.StatusOK, <-held, "the CLI is handed the answer")
}
