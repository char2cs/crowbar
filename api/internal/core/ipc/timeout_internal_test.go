package ipc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The per-request budget is white-box on purpose: it is a field on an unexported
// http.Client, and the only alternative way to observe it is to make a request
// against a server that deliberately stalls — a test that measures a clock
// instead of asserting a decision.
//
// What it guards is the pairing. The 5s default is correct for what it was
// written for — a hook or handoff callback holding a vendor CLI open — and wrong
// for the MCP relay, whose daemon-side work is git bounded at the daemon's own
// 60s ceiling. Raising the default to fix the relay would slacken every in-PTY
// callback with it, so the two budgets have to stay separately stated.

func TestNewClient_KeepsTheShortDefaultForInPTYCallbacks(t *testing.T) {
	c, err := NewClient("unix:///tmp/crowbar-timeout-test.sock")
	require.NoError(t, err)
	require.Equal(t, DefaultTimeout, c.http.Timeout)
	require.Equal(t, 5*time.Second, DefaultTimeout,
		"raising this raises it for every hook and handoff callback too; give the "+
			"caller that needs more its own budget instead")
}

func TestNewClientWithTimeout_UsesTheBudgetItIsGiven(t *testing.T) {
	c, err := NewClientWithTimeout("unix:///tmp/crowbar-timeout-test.sock", 120*time.Second)
	require.NoError(t, err)
	require.Equal(t, 120*time.Second, c.http.Timeout)
}
