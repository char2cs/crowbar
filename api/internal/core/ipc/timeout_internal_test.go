package ipc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

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
