//go:build integration

package mcp

import (
	"context"
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestMCP(t *testing.T) { suite.Run(t, new(MCPSuite)) }

type MCPSuite struct{ kit.IntegrationSuite }

// TestUnknownTokenReturnsError verifies that a missing or unknown token
// produces an authorization error from the MCP tool.
func (s *MCPSuite) TestUnknownTokenReturnsError() {
	mcp := s.Env.MCPClient.WithTest(s.T()).WithToken("definitely-not-a-real-token")
	err := mcp.Signal("user_approved", "")
	assert.Error(s.T(), err)
}

// TestCompletedRunTokenReturnsError verifies that a token for a completed run
// is rejected.
func (s *MCPSuite) TestCompletedRunTokenReturnsError() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	kit.DefaultTask(env.Client, repo.ID, "mcp-completed-token-branch")

	token := env.Stub.WaitForRunT(s.T(), ctx)
	// Complete the run by signalling.
	require.NoError(s.T(), env.MCPClient.WithTest(s.T()).WithToken(token).Signal("user_approved", ""))

	// After the run completes, the same token should be rejected.
	// Wait briefly for the status to update.
	time.Sleep(200 * time.Millisecond)

	err := env.MCPClient.WithTest(s.T()).WithToken(token).Signal("user_approved", "")
	assert.Error(s.T(), err, "expected error for completed run token")
}

// TestToolNotInStateForbidden verifies that calling a tool not in the current
// state's tool list returns a forbidden error.
func (s *MCPSuite) TestToolNotInStateForbidden() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	kit.DefaultTask(env.Client, repo.ID, "mcp-tool-forbidden-branch")

	// brainstorming state does not have crowbar_create_item.
	token := env.Stub.WaitForRunT(s.T(), ctx)
	mcp := env.MCPClient.WithTest(s.T()).WithToken(token)

	_, err := mcp.CreateItem("should be forbidden")
	assert.Error(s.T(), err, "expected forbidden error for create_item in brainstorming")
}

// TestInvalidEventStructuredError verifies that an unknown event name returns
// an error (not a panic or 500).
func (s *MCPSuite) TestInvalidEventStructuredError() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	kit.DefaultTask(env.Client, repo.ID, "mcp-invalid-event-branch")

	token := env.Stub.WaitForRunT(s.T(), ctx)
	mcp := env.MCPClient.WithTest(s.T()).WithToken(token)

	err := mcp.Signal("this_event_does_not_exist", "")
	assert.Error(s.T(), err, "expected error for unknown event")
}

// TestValidSignalAdvancesTaskAndPushesFrame verifies that a valid crowbar_signal
// advances the task state and delivers a state_transition ChatFrame.
func (s *MCPSuite) TestValidSignalAdvancesTaskAndPushesFrame() {
	env := s.Env
	ctx := context.Background()

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "mcp-valid-signal-branch")

	chatConn, closeChat := env.WSClient.ConnectChat(task.ID)
	defer closeChat()

	token := env.Stub.WaitForRunT(s.T(), ctx)
	require.NoError(s.T(), env.MCPClient.WithTest(s.T()).WithToken(token).Signal("user_approved", "done"))

	// Expect a state_transition frame on the chat WebSocket.
	waitChatFrame(s.T(), chatConn.Frames(), agent.ChatFrameTypeStateTransition, 3*time.Second)

	// Assert task advanced.
	updated := env.Client.GetTask(task.ID)
	assert.Equal(s.T(), "spec", updated.CurrentState)
}

// waitChatFrame blocks until a ChatFrame of the given type is received.
func waitChatFrame(
	t *testing.T,
	ch <-chan agent.ChatFrame,
	wantType agent.ChatFrameType,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("waitChatFrame: timeout waiting for frame type %q", wantType)
			return
		}
		select {
		case <-time.After(remaining):
			t.Fatalf("waitChatFrame: timeout waiting for frame type %q", wantType)
			return
		case f, ok := <-ch:
			if !ok {
				t.Fatalf("waitChatFrame: channel closed")
				return
			}
			if f.Type == wantType {
				return
			}
		}
	}
}
