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

// TestSpeaker_NeverRendersANonUserTurnAsTheUser: this rendering is what
// get_chat_log hands another agent and what a handoff hands an incoming CLI, and
// both read it as a transcript of a conversation with a person. A row whose
// sentences the human did not write has to say so on the line itself.
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
			// Crowbar's own observation about the chat. It renders as itself, which
			// is already not the word "user" — the property that matters.
			"notice",
			chatlog.Turn{Role: "notice", Provider: "claude"},
			"notice",
		},
		{
			// A role minted by a newer daemon than this code. It degrades to a label
			// a reader can look up, and it can never be "user".
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
