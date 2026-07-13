//go:build integration

package tests

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentProvidersEndpoint proves GET .../agent/providers returns the
// enumerated providers (id/displayName/icon) so the FE can render the row glyph,
// the New-chat rows, and the switch menu without N per-chat fetches.
func TestRegression_AgentProvidersEndpoint(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h) // adds the "stub" provider on disk
	imported := importWritableWorkspace(t, h)

	var providers []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Icon        string `json:"icon"`
	}
	h.get(wsBase(imported)+"/agent/providers", &providers)

	ids := map[string]bool{}
	for _, p := range providers {
		ids[p.ID] = true
	}
	require.True(t, ids["claude"], "claude is an embedded provider")
	require.True(t, ids["codex"], "codex is an embedded provider")
	assert.True(t, ids["stub"], "an on-disk provider is also enumerated")
}
