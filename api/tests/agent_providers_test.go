//go:build integration

package tests

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegression_AgentProvidersEndpoint(t *testing.T) {
	h := newHarness(t)
	writeStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	var providers []struct {
		ID          string `json:"id"`
		DisplayName string `json:"displayName"`
		Icon        string `json:"icon"`
	}
	h.get(repoBase(imported)+"/chats/providers", &providers)

	ids := map[string]bool{}
	for _, p := range providers {
		ids[p.ID] = true
	}
	require.True(t, ids["claude"], "claude is an embedded provider")
	require.True(t, ids["codex"], "codex is an embedded provider")
	assert.True(t, ids["stub"], "an on-disk provider is also enumerated")
}

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

func TestRegression_DisabledProviderIsRefusedNotJustHidden(t *testing.T) {
	h := newHarness(t)
	writeLiveStubProviderDescriptor(t, h)
	imported := importWritableWorkspace(t, h)

	putProviderPrefs(t, h, providerPref{ID: "livestub", Disabled: true})

	var providers []struct {
		ID      string `json:"id"`
		Enabled bool   `json:"enabled"`
	}
	h.get(repoBase(imported)+"/chats/providers", &providers)
	require.Contains(t, providerEnabled(providers), "livestub",
		"the disabled provider is still enumerated, which is why hiding it was never enough")
	assert.False(t, providerEnabled(providers)["livestub"], "and it is reported as switched off")
	assert.True(t, providerEnabled(providers)["claude"],
		"an untouched provider stays enabled, so the flag above is a real value")

	resp := h.raw(http.MethodPost, repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": imported.workspaceID}, http.StatusBadRequest)
	_ = resp.Body.Close()

	var chats []agentChatDTO
	h.get(repoBase(imported)+"/chats", &chats)
	assert.Empty(t, conversationsOnly(chats), "a refused spawn must not leave a chat behind")

	putProviderPrefs(t, h, providerPref{ID: "livestub", Disabled: false})
	var created struct {
		ID string `json:"id"`
	}
	h.post(repoBase(imported)+"/chats",
		map[string]string{"provider": "livestub", "workspaceId": imported.workspaceID}, http.StatusCreated, &created)
	require.NotEmpty(t, created.ID, "an enabled provider must still spawn")
}

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
