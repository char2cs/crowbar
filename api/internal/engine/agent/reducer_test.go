package agent_test

import (
	"testing"

	"github.com/char2cs/crowbar/api/internal/engine/agent"
	"github.com/stretchr/testify/require"
)

func TestDecide(t *testing.T) {
	tests := []struct {
		name                      string
		current, announced, known string
		isKnown                   bool
		want                      agent.Decision
	}{
		{"same conversation is a no-op", "s1", "s1", "", false,
			agent.Decision{Kind: agent.MoveNoop}},
		{"first announcement binds", "", "s1", "", false,
			agent.Decision{Kind: agent.MoveBind}},
		{"unknown new id mints a chat (/clear, /new)", "s1", "s2", "", false,
			agent.Decision{Kind: agent.MoveToNew}},
		{"known id goes to its chat (/resume)", "s1", "s2", "c2", true,
			agent.Decision{Kind: agent.MoveToKnown, ChatID: "c2"}},
		{"first announcement of a KNOWN id still binds in place", "", "s1", "c1", true,
			agent.Decision{Kind: agent.MoveBind}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, tc.want, agent.Decide(tc.current, tc.announced, tc.known, tc.isKnown))
		})
	}
}

// TestDecide_IsSourceAgnosticByConstruction LOCKS IN spec §3. Claude reports
// source=clear, Codex reports source=startup for the SAME event (verified
// against the real binaries). Decide takes no `source` argument at all — this
// test exists so nobody adds one.
func TestDecide_IsSourceAgnosticByConstruction(t *testing.T) {
	// The signature has no `source` parameter. If this file stops compiling
	// because someone added one, that is the bug.
	got := agent.Decide("s1", "s2", "", false)
	require.Equal(t, agent.MoveToNew, got.Kind)
}
