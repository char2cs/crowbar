package chatlog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/chatlog"
)

// The record has more than one consumer — the chat surface and the cross-agent
// tool surface — and two spellings would make one conversation read as two
// different ones depending on who asked.
func TestSpeaker_AttributesAVendorReplyToItsProvider(t *testing.T) {
	testCases := []struct {
		name string
		turn chatlog.Turn
		want string
	}{
		{"user", chatlog.Turn{Role: "user"}, "user"},
		{"user is never provider-tagged", chatlog.Turn{Role: "user", Provider: "claude"}, "user"},
		{"assistant names its provider", chatlog.Turn{Role: "assistant", Provider: "codex"}, "assistant (codex)"},
		{"assistant with no provider", chatlog.Turn{Role: "assistant"}, "assistant"},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.turn.Speaker())
		})
	}
}
