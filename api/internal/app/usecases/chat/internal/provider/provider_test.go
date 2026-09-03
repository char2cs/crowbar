package provider_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// newTable builds the provider table over the SHIPPED descriptors and a real
// (in-memory) preference store. The descriptors are the point: a stub catalogue
// would let the ordering rules agree with a fiction.
func newTable(t *testing.T, installed func(engineagents.Agent) bool) *provider.Providers {
	t.Helper()

	prefs, err := storesqlite.New[domain.AgentProviderPreference, string](":memory:")
	require.NoError(t, err)

	home := t.TempDir()
	return provider.New(provider.Deps{
		Agents:    engineagents.New(),
		Home:      func() (string, error) { return home, nil },
		Installed: installed,
		Prefs:     prefs,
	})
}

func ids(providers []domain.AgentProvider) []string {
	out := make([]string, 0, len(providers))
	for _, p := range providers {
		out = append(out, p.ID)
	}
	return out
}

func TestResolveProviders_ListsTheShippedDescriptors(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })

	providers, err := table.ResolveProviders(t.Context())

	require.NoError(t, err)
	assert.Contains(t, ids(providers), "claude")
	assert.Contains(t, ids(providers), "codex")
	for _, p := range providers {
		assert.True(t, p.Enabled, "a provider with no preference row is enabled")
		assert.True(t, p.MCPEnabled, "and may use the tool surface")
		assert.False(t, p.Connected, "the install probe said otherwise")
	}
}

// Connected is the install probe's answer, not a stored flag. It is injected so
// this never depends on the host having claude or codex on its PATH.
func TestResolveProviders_ConnectedComesFromTheInstallProbe(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(a engineagents.Agent) bool { return a.ID() == "codex" })

	providers, err := table.ResolveProviders(t.Context())

	require.NoError(t, err)
	for _, p := range providers {
		assert.Equal(t, p.ID == "codex", p.Connected, "provider %s", p.ID)
	}
}

// A nil probe must default to the REAL one rather than reporting everything
// disconnected: that is the shape the composition root passes.
func TestNew_NilInstallProbeDefaultsToTheRealOne(t *testing.T) {
	t.Parallel()

	table := newTable(t, nil)

	_, err := table.ResolveProviders(t.Context())

	require.NoError(t, err)
}

// Preferenced providers sort before unpreferenced ones, then by priority, then by
// id. The last tiebreak is what makes the order stable across restarts.
func TestResolveProviders_OrdersPreferencedFirstThenByPriorityThenByID(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })
	_, err := table.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "codex", Priority: 1},
	})
	require.NoError(t, err)

	providers, err := table.ResolveProviders(t.Context())

	require.NoError(t, err)
	require.NotEmpty(t, providers)
	assert.Equal(t, "codex", providers[0].ID,
		"the only preferenced provider sorts ahead of every unpreferenced one")
}

func TestReplaceProviderPreferences_ReportsTheResolvedTable(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })

	providers, err := table.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "claude", Disabled: true, MCPDisabled: true},
	})

	require.NoError(t, err)
	for _, p := range providers {
		if p.ID != "claude" {
			continue
		}
		assert.False(t, p.Enabled)
		assert.False(t, p.MCPEnabled)
	}
}

// A provider the submission omits reverts to default. Without the delete-first
// pass it would linger with the priority it had, and the user's reorder would be
// half applied.
func TestReplaceProviderPreferences_DropsOmittedRows(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })
	_, err := table.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "claude", Disabled: true},
	})
	require.NoError(t, err)

	_, err = table.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "codex", Priority: 1},
	})
	require.NoError(t, err)

	require.NoError(t, table.RequireProviderEnabled(t.Context(), "claude"),
		"claude's disabled row should have been dropped with the rest of the old table")
}

func TestReplaceProviderPreferences_RefusesAnUnknownProvider(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })

	_, err := table.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "telepathy"},
	})

	require.ErrorIs(t, err, apperr.ErrInvalidArgument)
}

// Disabled is a decision the table records and ResolveProviders reports, but
// reporting is not enforcing: a spawn names a provider id directly and never
// passes through the list that flag decorates.
func TestRequireProviderEnabled_RefusesADisabledProvider(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })
	_, err := table.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "claude", Disabled: true},
	})
	require.NoError(t, err)

	err = table.RequireProviderEnabled(t.Context(), "claude")

	require.ErrorIs(t, err, provider.ErrProviderDisabled)
	require.ErrorIs(t, err, apperr.ErrInvalidArgument, "handlers answer 400 off this class")
	assert.NoError(t, table.RequireProviderEnabled(t.Context(), "codex"))
	assert.NoError(t, table.RequireProviderEnabled(t.Context(), "never-preferenced"),
		"a provider with no row is enabled")
}

// The tool switch is read LIVE, on every call. A chat spawned with tools on must
// lose them the moment the user turns the switch off in Settings.
func TestProviderMCPEnabled_FollowsThePreference(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })

	on, err := table.ProviderMCPEnabled(t.Context(), "claude")
	require.NoError(t, err)
	assert.True(t, on, "a provider with no row may use the tool surface")

	_, err = table.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "claude", MCPDisabled: true},
	})
	require.NoError(t, err)

	on, err = table.ProviderMCPEnabled(t.Context(), "claude")
	require.NoError(t, err)
	assert.False(t, on)
}

// A wiring mistake has to be LOUD: an agent that silently has no tools looks
// identical to an agent that chose not to use them.
func TestDispatchMCP_RefusesAnUnconfiguredToolSurface(t *testing.T) {
	t.Parallel()

	table := provider.New(provider.Deps{})

	_, send, err := table.DispatchMCP(t.Context(), "runner-1", "token",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))

	require.Error(t, err)
	assert.False(t, send)
}
