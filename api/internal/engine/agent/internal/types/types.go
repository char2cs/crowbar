// Package types defines the shared ChatFrame types used by the agent package
// and its internal sub-packages. It is a leaf package to avoid import cycles.
package types

// ChatFrameType identifies the kind of payload carried by a ChatFrame.
type ChatFrameType string

const (
	ChatFrameTypeUserMessage     ChatFrameType = "user_message"
	ChatFrameTypeAgentChunk      ChatFrameType = "agent_chunk"
	ChatFrameTypeAgentTurnEnd    ChatFrameType = "agent_turn_end"
	ChatFrameTypeToolCall        ChatFrameType = "tool_call"
	ChatFrameTypeToolResult      ChatFrameType = "tool_result"
	ChatFrameTypeStateTransition ChatFrameType = "state_transition"
)

// ChatFrame is a single event in the bidirectional chat stream between the
// frontend and a running ACP agent session.
type ChatFrame struct {
	Type      ChatFrameType `json:"type"`
	MessageID string        `json:"message_id,omitempty"`
	Delta     string        `json:"delta,omitempty"`
	Tool      string        `json:"tool,omitempty"`
	Args      any           `json:"args,omitempty"`
	Result    any           `json:"result,omitempty"`
	NewState  string        `json:"new_state,omitempty"`
	Content   string        `json:"content,omitempty"`
}
