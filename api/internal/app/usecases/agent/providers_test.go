package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// geminiDescriptor is a minimal-valid on-disk descriptor override, written into a
// fixture home so ResolveProviders sees a THIRD provider (beyond the embedded
// claude/codex) with no stored preference — the "appended by id, enabled by
// default" case.
const geminiDescriptor = `id: gemini
display_name: Gemini
icon: '<svg/>'
spawn:
  cmd: gemini
  interactive_required: true
hooks:
  format: json
  events:
    session_start:
      session_id: session_id
    turn_stop:
      message: last_assistant_message
`

func writeGeminiDescriptor(
	t *testing.T,
	home string,
) {
	t.Helper()
	dir := filepath.Join(home, "descriptors")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "gemini.yaml"), []byte(geminiDescriptor), 0o644))
}

func providerIDs(
	got []dto.AgentProviderDTO,
) []string {
	ids := make([]string, len(got))
	for i, p := range got {
		ids[i] = p.ID
	}
	return ids
}

// TestResolveProviders_OrdersByPreferenceThenAppendsNewEnabled pins the whole
// resolution contract: preferenced providers come first in saved priority order,
// unpreferenced descriptors are appended by id, enabled reflects !disabled, and a
// provider with no row defaults to enabled. Connected is the injected probe's
// verdict, independent of the host.
func TestResolveProviders_OrdersByPreferenceThenAppendsNewEnabled(t *testing.T) {
	f := newFixture(t)
	writeGeminiDescriptor(t, f.ws.home)

	f.setPrefs(t,
		domain.AgentProviderPreference{ProviderID: "codex", Priority: 0, Disabled: false},
		domain.AgentProviderPreference{ProviderID: "claude", Priority: 1, Disabled: true},
	)
	f.setConnected(map[string]bool{"codex": true, "claude": false, "gemini": true})

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"codex", "claude", "gemini"}, providerIDs(got),
		"preferenced first in priority order, unpreferenced appended by id")
	assert.True(t, got[0].Enabled, "codex enabled")
	assert.True(t, got[0].Connected, "codex installed")
	assert.False(t, got[1].Enabled, "claude disabled")
	assert.False(t, got[1].Connected, "claude not installed")
	assert.True(t, got[2].Enabled, "gemini defaults to enabled")
	assert.True(t, got[2].Connected, "gemini installed")
}

// TestResolveProviders_NoPreferences_OrdersByDescriptorID proves the empty-store
// default: with no stored preferences every embedded descriptor is enabled and
// ordered by id.
func TestResolveProviders_NoPreferences_OrdersByDescriptorID(t *testing.T) {
	f := newFixture(t)
	f.setConnected(map[string]bool{"claude": true, "codex": true})

	got, err := f.usecase.ResolveProviders(f.ctx)
	require.NoError(t, err)

	assert.Equal(t, []string{"claude", "codex"}, providerIDs(got))
	for _, p := range got {
		assert.True(t, p.Enabled, "%s defaults to enabled", p.ID)
	}
}

// A disabled provider is a provider the user has switched OFF, and the only
// place that decision can be honoured is the spawn path: the preference is
// persisted and reported, but a POST .../agent/chats naming it — from a stale
// tab, a second window, or the CLI — reaches spawnRunner directly and never
// passes through the list the Enabled flag decorates.
func TestSpawnChat_RefusesDisabledProvider(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", Disabled: true})

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "codex")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument,
		"a disabled provider must be refused with a 4xx-mapping sentinel")
	assert.Zero(t, f.term.callCount(), "no vendor CLI may be launched for a disabled provider")
}

// The guard is scoped to the provider that is actually off: disabling one must
// not take the others down with it.
func TestSpawnChat_AllowsEnabledProviderAlongsideADisabledOne(t *testing.T) {
	f := newFixture(t)
	f.setPrefs(t,
		domain.AgentProviderPreference{ProviderID: "codex", Disabled: true},
		domain.AgentProviderPreference{ProviderID: "claude", Disabled: false},
	)

	_, _, err := f.usecase.SpawnChat(f.ctx, "ws1", "claude")

	require.NoError(t, err)
	assert.Equal(t, 1, f.term.callCount())
}

// Switching an existing chat onto a disabled provider is the same decision by
// another route, and it is the worse one to leave open: the switch QUITS the
// live CLI before it spawns the replacement, so an unguarded switch onto a
// disabled provider would leave the chat with no agent at all.
func TestSwitchProvider_RefusesDisabledTarget(t *testing.T) {
	f := newFixture(t)
	chatID, _ := f.spawn(t, "claude")
	f.setPrefs(t, domain.AgentProviderPreference{ProviderID: "codex", Disabled: true})

	_, err := f.usecase.SwitchProvider(f.ctx, chatID, "codex")

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
	assert.Equal(t, 1, f.term.callCount(), "the disabled target must never be spawned")
	assert.Empty(t, f.term.terminateRequestIDs(),
		"a refused switch must not quit the CLI the chat still has")
}
