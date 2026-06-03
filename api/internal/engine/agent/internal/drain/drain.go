// Package drain translates ACP SessionNotification events into ChatFrames
// and writes a newline-delimited JSON event log.
package drain

import (
	"context"
	"encoding/json"
	"io"

	acp "github.com/coder/acp-go-sdk"

	"github.com/char2cs/crowbar/api/internal/engine/agent/internal/types"
)

// Run reads from updates until it is closed or ctx is cancelled.
// Each notification is written as a JSON line to dest and, when relevant,
// mapped to a ChatFrame that is delivered via publish.
// publish must be non-blocking; frames are dropped if the caller cannot keep up.
func Run(
	ctx context.Context,
	updates <-chan acp.SessionNotification,
	dest io.WriteCloser,
	publish func(types.ChatFrame),
) error {
	enc := json.NewEncoder(dest)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case n, ok := <-updates:
			if !ok {
				return nil
			}
			_ = enc.Encode(n)
			frame, mapped := toFrame(n)
			if mapped {
				publish(frame)
			}
		}
	}
}

func toFrame(
	n acp.SessionNotification,
) (types.ChatFrame, bool) {
	u := n.Update
	switch {
	case u.AgentMessageChunk != nil:
		chunk := u.AgentMessageChunk
		if chunk.Content.Text == nil {
			return types.ChatFrame{}, false
		}
		msgID := ""
		if chunk.MessageId != nil {
			msgID = *chunk.MessageId
		}
		return types.ChatFrame{
			Type:      types.ChatFrameTypeAgentChunk,
			MessageID: msgID,
			Delta:     chunk.Content.Text.Text,
		}, true

	case u.ToolCall != nil:
		tc := u.ToolCall
		return types.ChatFrame{
			Type:      types.ChatFrameTypeToolCall,
			Tool:      tc.Title,
			MessageID: string(tc.ToolCallId),
			Args:      tc.RawInput,
		}, true

	case u.ToolCallUpdate != nil:
		tcu := u.ToolCallUpdate
		if tcu.Status == nil {
			return types.ChatFrame{}, false
		}
		return types.ChatFrame{
			Type:      types.ChatFrameTypeToolResult,
			MessageID: string(tcu.ToolCallId),
			Result:    tcu.RawOutput,
		}, true

	default:
		return types.ChatFrame{}, false
	}
}
