//go:build integration

package chat

import (
	"testing"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/char2cs/crowbar/api/tests/kit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestChat(t *testing.T) { suite.Run(t, new(ChatSuite)) }

type ChatSuite struct{ kit.IntegrationSuite }

// TestBidirectionalChat sends a user message, asserts it is written to SQLite,
// then streams agent chunks and asserts the assembled message is also persisted.
func (s *ChatSuite) TestBidirectionalChat() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "chat-test-branch")

	chatConn, closeChat := env.WSClient.ConnectChat(task.ID)
	defer closeChat()

	// Send a user message.
	require.NoError(s.T(), chatConn.Send("hello"))

	// The user message should now be in SQLite.
	waitForMessages(s.T(), func() bool {
		msgs := env.Client.GetMessages(task.ID)
		for _, m := range msgs {
			if m.Role == "user" && m.Content == "hello" {
				return true
			}
		}
		return false
	}, 3*time.Second)

	// Stream two chunks from the stub; it should aggregate and write to SQLite.
	env.Stub.StreamChunks("hello ", "world")

	// Expect two agent_chunk frames and one agent_turn_end.
	frames := collectFrames(s.T(), chatConn.Frames(), 3, 3*time.Second)
	assert.Equal(s.T(), string(agent.ChatFrameTypeAgentChunk), string(frames[0].Type))
	assert.Equal(s.T(), "hello ", frames[0].Delta)
	assert.Equal(s.T(), string(agent.ChatFrameTypeAgentChunk), string(frames[1].Type))
	assert.Equal(s.T(), "world", frames[1].Delta)
	assert.Equal(s.T(), string(agent.ChatFrameTypeAgentTurnEnd), string(frames[2].Type))

	// The agent turn should be persisted.
	waitForMessages(s.T(), func() bool {
		msgs := env.Client.GetMessages(task.ID)
		for _, m := range msgs {
			if m.Role == "agent" && m.Content == "hello world" {
				return true
			}
		}
		return false
	}, 3*time.Second)

	// GET /messages returns both in chronological order.
	msgs := env.Client.GetMessages(task.ID)
	require.GreaterOrEqual(s.T(), len(msgs), 2)
	assert.Equal(s.T(), "user", string(msgs[0].Role))
	assert.Equal(s.T(), "agent", string(msgs[1].Role))
}

// TestReconnectHistory verifies that a new chat connection receives the full
// history via the REST endpoint.
func (s *ChatSuite) TestReconnectHistory() {
	env := s.Env

	gitRepo := kit.NewGitRepo(s.T())
	project := kit.DefaultProject(env.Client)
	repo := kit.DefaultRepository(env.Client, project.ID, gitRepo.Path())
	task := kit.DefaultTask(env.Client, repo.ID, "chat-reconnect-branch")

	chatConn, closeChat := env.WSClient.ConnectChat(task.ID)
	require.NoError(s.T(), chatConn.Send("first message"))
	closeChat()

	// Reconnect and load history via REST.
	msgs := env.Client.GetMessages(task.ID)
	assert.GreaterOrEqual(s.T(), len(msgs), 1)
	assert.Equal(s.T(), "first message", msgs[0].Content)
}

// waitForMessages polls fn until it returns true or timeout elapses.
func waitForMessages(
	t *testing.T,
	fn func() bool,
	timeout time.Duration,
) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	t.Fatal("waitForMessages: condition not met within timeout")
}

// collectFrames reads exactly n frames from ch within timeout.
func collectFrames(
	t *testing.T,
	ch <-chan agent.ChatFrame,
	n int,
	timeout time.Duration,
) []agent.ChatFrame {
	t.Helper()
	var frames []agent.ChatFrame
	deadline := time.Now().Add(timeout)
	for len(frames) < n {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			t.Fatalf("collectFrames: timeout; got %d/%d frames", len(frames), n)
		}
		select {
		case f, ok := <-ch:
			if !ok {
				t.Fatalf("collectFrames: channel closed; got %d/%d frames", len(frames), n)
			}
			frames = append(frames, f)
		case <-time.After(remaining):
			t.Fatalf("collectFrames: timeout; got %d/%d frames", len(frames), n)
		}
	}
	return frames
}
