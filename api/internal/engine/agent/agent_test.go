package agent

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/core/config"
)

// TestChatFrameType_constants verifies each constant has the expected string value.
func TestChatFrameType_constants(t *testing.T) {
	assert.Equal(t, ChatFrameType("user_message"), ChatFrameTypeUserMessage)
	assert.Equal(t, ChatFrameType("agent_chunk"), ChatFrameTypeAgentChunk)
	assert.Equal(t, ChatFrameType("agent_turn_end"), ChatFrameTypeAgentTurnEnd)
	assert.Equal(t, ChatFrameType("tool_call"), ChatFrameTypeToolCall)
	assert.Equal(t, ChatFrameType("tool_result"), ChatFrameTypeToolResult)
	assert.Equal(t, ChatFrameType("state_transition"), ChatFrameTypeStateTransition)
}

// TestChatFrame_zeroValue ensures ChatFrame is instantiable and all fields are
// addressable — this instruments every field declaration in the struct.
func TestChatFrame_zeroValue(t *testing.T) {
	var f ChatFrame
	assert.Equal(t, ChatFrameType(""), f.Type)
	assert.Equal(t, "", f.MessageID)
	assert.Equal(t, "", f.Delta)
	assert.Equal(t, "", f.Tool)
	assert.Nil(t, f.Args)
	assert.Nil(t, f.Result)
	assert.Equal(t, "", f.NewState)
	assert.Equal(t, "", f.Content)
}

// TestChatFrame_literalInit ensures a composite literal works correctly.
func TestChatFrame_literalInit(t *testing.T) {
	f := ChatFrame{
		Type:      ChatFrameTypeAgentChunk,
		MessageID: "msg-1",
		Delta:     "hello",
		Tool:      "bash",
		Args:      map[string]string{"cmd": "ls"},
		Result:    42,
		NewState:  "coding",
		Content:   "full content",
	}

	assert.Equal(t, ChatFrameTypeAgentChunk, f.Type)
	assert.Equal(t, "msg-1", f.MessageID)
	assert.Equal(t, "hello", f.Delta)
	assert.Equal(t, "bash", f.Tool)
	assert.Equal(t, map[string]string{"cmd": "ls"}, f.Args)
	assert.Equal(t, 42, f.Result)
	assert.Equal(t, "coding", f.NewState)
	assert.Equal(t, "full content", f.Content)
}

// TestAgentRuntime_isInterface verifies that AgentRuntime is an interface by
// confirming that a nil pointer assignment compiles.
func TestAgentRuntime_isInterface(t *testing.T) {
	var _ AgentRuntime = (AgentRuntime)(nil)
}

type testHub struct{}

func (h *testHub) RegisterSession(_ string) (<-chan string, func(ChatFrame), func()) {
	return make(chan string), func(ChatFrame) {}, func() {}
}

func TestNew_ReturnsNonNil(t *testing.T) {
	hub := &testHub{}
	rt := New(config.IntelligenceConfig{}, func(id string) string { return id }, hub, "", "")
	assert.NotNil(t, rt)
}
