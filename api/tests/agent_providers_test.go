//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestRegression_AgentProvidersEndpoint proves GET .../chats/providers returns the
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
	h.get(wsBase(imported)+"/chats/providers", &providers)

	ids := map[string]bool{}
	for _, p := range providers {
		ids[p.ID] = true
	}
	require.True(t, ids["claude"], "claude is an embedded provider")
	require.True(t, ids["codex"], "codex is an embedded provider")
	assert.True(t, ids["stub"], "an on-disk provider is also enumerated")
}

// providerPref is one entry of the PUT /v0/settings/chat/providers body: the
// full ordered set, where the array position is the provider's priority.
type providerPref struct {
	ID       string `json:"id"`
	Disabled bool   `json:"disabled"`
}

func putProviderPrefs(
	t *testing.T,
	h *harness,
	prefs ...providerPref,
) {
	t.Helper()
	var out []map[string]any
	h.put("/v0/settings/chat/providers", map[string]any{"providers": prefs}, &out)
}

// TestRegression_DisabledProviderIsRefusedNotJustHidden pins a preference that
// was persisted and reported but never enforced: Disabled was read only by the
// handler that binds it and the resolver that reports Enabled, so no spawn path
// consulted it. A POST .../chats naming a disabled provider — from a stale
// tab, a second window, or the CLI — launched it exactly as if it were on.
func TestRegression_DisabledProviderIsRefusedNotJustHidden(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	putProviderPrefs(t, h, providerPref{ID: "livestub", Disabled: true})

	// The catalog still reports it, switched off — that half always worked.
	var providers []struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	h.get(wsBase(imported)+"/chats/providers", &providers)
	require.Contains(t, providerEnabled(providers), "livestub",
		"the disabled provider is still enumerated, which is why hiding it was never enough")
	assert.False(t, providerEnabled(providers)["livestub"], "and it is reported as switched off")
	assert.True(t, providerEnabled(providers)["claude"],
		"an untouched provider stays enabled, so the flag above is a real value")

	resp := h.raw(http.MethodPost, wsBase(imported)+"/chats",
		map[string]string{"provider": "livestub"}, http.StatusBadRequest)
	_ = resp.Body.Close()

	var chats []agentChatDTO
	h.get(wsBase(imported)+"/chats", &chats)
	assert.Empty(t, chats, "a refused spawn must not leave a chat behind")

	// Re-enabling it makes the very same request work, so the guard is the
	// preference and nothing else.
	putProviderPrefs(t, h, providerPref{ID: "livestub", Disabled: false})
	var created struct {
		ID string `json:"id"`
	}
	h.post(wsBase(imported)+"/chats",
		map[string]string{"provider": "livestub"}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "an enabled provider must still spawn")
}

// providerEnabled indexes an enumerated provider list by id → enabled.
func providerEnabled(
	got []struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	},
) map[string]bool {
	out := make(map[string]bool, len(got))
	for _, p := range got {
		out[p.ID] = p.Enabled
	}
	return out
}
