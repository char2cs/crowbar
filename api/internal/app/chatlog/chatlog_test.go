package chatlog_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/app/chatlog"
)

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

func TestSpeaker_NeverRendersANonUserTurnAsTheUser(t *testing.T) {
	testCases := []struct {
		name string
		turn chatlog.Turn
		want string
	}{
		{
			"harness names the provider whose machinery wrote it",
			chatlog.Turn{Role: "harness", Provider: "claude"},
			"claude harness (injected, NOT the user)",
		},
		{
			"harness with no provider still disclaims",
			chatlog.Turn{Role: "harness"},
			"harness (injected, NOT the user)",
		},
		{
			"notice",
			chatlog.Turn{Role: "notice", Provider: "claude"},
			"notice",
		},
		{
			"an unknown role renders as itself",
			chatlog.Turn{Role: "summary", Provider: "claude"},
			"summary",
		},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.turn.Speaker())
			assert.NotEqual(t, "user", tc.turn.Speaker())
		})
	}
}
