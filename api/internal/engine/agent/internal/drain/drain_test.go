package drain_test

import (
	"bytes"
	"context"
	"io"
	"testing"

	acp "github.com/coder/acp-go-sdk"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/drain"
	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/types"
)

type nopCloser struct{ io.Writer }

func (nopCloser) Close() error { return nil }

func collectFrames(
	t *testing.T,
	notifications []acp.SessionNotification,
) ([]types.ChatFrame, string) {
	t.Helper()
	ch := make(chan acp.SessionNotification, len(notifications))
	for _, n := range notifications {
		ch <- n
	}
	close(ch)

	var buf bytes.Buffer
	var frames []types.ChatFrame
	publish := func(f types.ChatFrame) {
		frames = append(frames, f)
	}

	err := drain.Run(context.Background(), ch, nopCloser{&buf}, publish)
	require.NoError(t, err)
	return frames, buf.String()
}

func TestRun_agentMessageChunk(
	t *testing.T,
) {
	msgID := "msg-1"
	n := acp.SessionNotification{
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content:   acp.TextBlock("hello"),
				MessageId: &msgID,
			},
		},
	}
	frames, log := collectFrames(t, []acp.SessionNotification{n})

	require.Len(t, frames, 1)
	assert.Equal(t, types.ChatFrameTypeAgentChunk, frames[0].Type)
	assert.Equal(t, "hello", frames[0].Delta)
	assert.Equal(t, "msg-1", frames[0].MessageID)
	assert.Contains(t, log, "agent_message_chunk")
}

func TestRun_toolCall(
	t *testing.T,
) {
	n := acp.SessionNotification{
		Update: acp.StartToolCall("tc-1", "Read file"),
	}
	frames, _ := collectFrames(t, []acp.SessionNotification{n})

	require.Len(t, frames, 1)
	assert.Equal(t, types.ChatFrameTypeToolCall, frames[0].Type)
	assert.Equal(t, "Read file", frames[0].Tool)
}

func TestRun_toolCallUpdate_withStatus(
	t *testing.T,
) {
	status := acp.ToolCallStatusCompleted
	n := acp.SessionNotification{
		Update: acp.UpdateToolCall("tc-1", acp.WithUpdateStatus(status), acp.WithUpdateRawOutput("output")),
	}
	frames, _ := collectFrames(t, []acp.SessionNotification{n})

	require.Len(t, frames, 1)
	assert.Equal(t, types.ChatFrameTypeToolResult, frames[0].Type)
	assert.Equal(t, "output", frames[0].Result)
}

func TestRun_toolCallUpdate_noStatus_skipped(
	t *testing.T,
) {
	n := acp.SessionNotification{
		Update: acp.UpdateToolCall("tc-1"),
	}
	frames, _ := collectFrames(t, []acp.SessionNotification{n})
	assert.Empty(t, frames)
}

func TestRun_contextCancel(
	t *testing.T,
) {
	ch := make(chan acp.SessionNotification)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := drain.Run(ctx, ch, nopCloser{&bytes.Buffer{}}, func(types.ChatFrame) {})
	require.Error(t, err)
}

func TestRun_channelClose_noError(
	t *testing.T,
) {
	ch := make(chan acp.SessionNotification)
	close(ch)
	err := drain.Run(context.Background(), ch, nopCloser{&bytes.Buffer{}}, func(types.ChatFrame) {})
	require.NoError(t, err)
}

func TestRun_agentMessageChunk_nilText_skipped(
	t *testing.T,
) {
	n := acp.SessionNotification{
		Update: acp.UpdateAgentMessage(acp.ImageBlock("base64data", "image/png")),
	}
	frames, _ := collectFrames(t, []acp.SessionNotification{n})
	assert.Empty(t, frames)
}

func TestRun_agentMessageChunk_nilMessageID(
	t *testing.T,
) {
	n := acp.SessionNotification{
		Update: acp.SessionUpdate{
			AgentMessageChunk: &acp.SessionUpdateAgentMessageChunk{
				Content: acp.TextBlock("hello"),
			},
		},
	}
	frames, _ := collectFrames(t, []acp.SessionNotification{n})

	require.Len(t, frames, 1)
	assert.Equal(t, "", frames[0].MessageID)
	assert.Equal(t, "hello", frames[0].Delta)
}

func TestRun_unknownUpdate_skipped(
	t *testing.T,
) {
	n := acp.SessionNotification{
		Update: acp.SessionUpdate{},
	}
	frames, _ := collectFrames(t, []acp.SessionNotification{n})
	assert.Empty(t, frames)
}
