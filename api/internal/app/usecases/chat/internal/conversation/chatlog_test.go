package conversation

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// speaker is in-package, so this test is too. It is the one piece of the ledger
// that is DISPLAY rather than data, and the harness case is the reason it exists:
// an injected prompt rendered as "user" would let a model reading its own handoff
// mistake the harness for a person.
func TestSpeaker_AttributesAVendorReplyToItsProvider(t *testing.T) {
	for _, tc := range []struct {
		name string
		turn domain.LedgerTurn
		want string
	}{
		{"assistant with a provider", domain.LedgerTurn{Role: "assistant", Provider: "claude"}, "assistant (claude)"},
		{"assistant with no provider", domain.LedgerTurn{Role: "assistant"}, "assistant"},
		{"a user is just the user", domain.LedgerTurn{Role: "user", Provider: "claude"}, "user"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, speaker(tc.turn))
		})
	}
}

func TestSpeaker_NeverRendersANonUserTurnAsTheUser(t *testing.T) {
	for _, tc := range []struct {
		name string
		turn domain.LedgerTurn
		want string
	}{
		{"harness with a provider", domain.LedgerTurn{Role: "harness", Provider: "codex"}, "codex harness (injected, NOT the user)"},
		{"harness with no provider", domain.LedgerTurn{Role: "harness"}, "harness (injected, NOT the user)"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, speaker(tc.turn))
			assert.NotEqual(t, "user", speaker(tc.turn))
		})
	}
}
