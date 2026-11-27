package provider_test

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/provider"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/engine/agents/descriptorcheck"
)

func newTable(t *testing.T, installed func(engineagents.Agent) bool) *provider.Providers {
	t.Helper()
	return newTableAt(t, t.TempDir(), installed)
}

func newTableAt(t *testing.T, home string, installed func(engineagents.Agent) bool) *provider.Providers {
	t.Helper()

	prefs, err := storesqlite.New[domain.AgentProviderPreference, string](":memory:")
	require.NoError(t, err)

	return provider.New(provider.Deps{
		Agents:    engineagents.New(engineagents.WithOverrideCheck(descriptorcheck.AcceptOverride)),
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

func TestInstallDescriptor_AddsAProviderAndRejectsInvalidOrExisting(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	table := newTableAt(t, home, func(engineagents.Agent) bool { return false })
	shipped := shippedClaude(t)
	raw := []byte(strings.Replace(string(shipped), "id: claude", "id: customagent", 1))
	report, err := table.InstallDescriptor(t.Context(), raw)
	require.NoError(t, err)
	assert.Equal(t, "customagent", report.ID)
	providers, err := table.ResolveProviders(t.Context())
	require.NoError(t, err)
	assert.Contains(t, ids(providers), "customagent")
	_, err = table.InstallDescriptor(t.Context(), raw)
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	_, err = table.InstallDescriptor(t.Context(), shipped)
	require.ErrorIs(t, err, problem.ErrInvalidArgument)
	_, err = table.InstallDescriptor(t.Context(), []byte("id: broken\n"))
	require.ErrorIs(t, err, problem.ErrUnprocessable)
	_, err = os.Stat(filepath.Join(home, "descriptors", "broken.yaml"))
	assert.True(t, os.IsNotExist(err))
}

func TestInstallDescriptor_ServesAnUnsafeIconAsNone(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	table := newTableAt(t, home, func(engineagents.Agent) bool { return false })
	shipped := shippedClaude(t)
	raw := strings.Replace(string(shipped), "id: claude", "id: customagent", 1)
	raw = regexp.MustCompile(`(?m)^icon: .*$`).ReplaceAllString(raw,
		`icon: '<svg xmlns="http://www.w3.org/2000/svg"><foreignObject><img src="x" onerror="alert(1)"/></foreignObject></svg>'`)
	require.Contains(t, raw, "foreignObject")

	report, err := table.InstallDescriptor(t.Context(), []byte(raw))
	require.NoError(t, err)
	assert.Equal(t, "customagent", report.ID)

	providers, err := table.ResolveProviders(t.Context())
	require.NoError(t, err)
	var installed *domain.AgentProvider
	for i := range providers {
		if providers[i].ID == "customagent" {
			installed = &providers[i]
		}
	}
	require.NotNil(t, installed, "the provider must still be listed")
	assert.Empty(t, installed.Icon)
	require.NoError(t, table.RequireProviderEnabled(t.Context(), "customagent"), "an unsafe icon never blocks its provider")
}

func TestDescriptorReports_ReportsABrokenOverrideItRefusesToEnable(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", "codex.yaml"), []byte("id: codex\n"), 0o600))
	table := newTableAt(t, home, func(engineagents.Agent) bool { return false })

	reports, err := table.DescriptorReports(t.Context())
	require.NoError(t, err)
	byID := map[string]bool{}
	for _, r := range reports {
		byID[r.ID] = r.OK()
	}
	assert.False(t, byID["codex"], "the broken override is reported")
	assert.True(t, byID["claude"], "the other shipped descriptors still load")

	providers, err := table.ResolveProviders(t.Context())
	require.NoError(t, err)
	assert.Contains(t, ids(providers), "codex", "the shipped codex runs in its place")
	require.NoError(t, table.RequireProviderEnabled(t.Context(), "codex"))
}

func TestRequireProviderEnabled_RefusesADescriptorWithAnError(t *testing.T) {
	t.Parallel()
	table := newTableAt(t, blockedHome(t, "acme"), nil)

	err := table.RequireProviderEnabled(t.Context(), "acme")

	require.ErrorIs(t, err, descriptorcheck.ErrBlocked)
	require.ErrorIs(t, err, problem.ErrUnprocessable)
	assert.Contains(t, err.Error(), "is a shell")
	assert.NoError(t, table.RequireProviderEnabled(t.Context(), "codex"), "the shipped codex is untouched")
}

func TestResolveProviders_OffersNoProviderTheGateRefuses(t *testing.T) {
	t.Parallel()
	table := newTableAt(t, blockedHome(t, "aaa"), func(engineagents.Agent) bool { return true })

	got, err := table.ResolveProviders(t.Context())
	require.NoError(t, err)
	assert.NotContains(t, ids(got), "aaa")
	def, err := table.DefaultProviderID(t.Context())
	require.NoError(t, err)
	assert.NotEqual(t, "aaa", def)

	reports, err := table.DescriptorReports(t.Context())
	require.NoError(t, err)
	assert.True(t, slices.ContainsFunc(reports, func(r descriptorcheck.Report) bool { return r.ID == "aaa" && !r.OK() }),
		"the refused descriptor is still reported, with what to fix")
}

func blockedHome(t *testing.T, id string) string {
	t.Helper()
	home := t.TempDir()
	shell := strings.Replace(string(shippedClaude(t)), "  cmd: claude\n  interactive_required", "  cmd: /bin/sh\n  interactive_required", 1)
	shell = strings.Replace(shell, "\nid: claude\n", "\nid: "+id+"\n", 1)
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o750))
	require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", id+".yaml"), []byte(shell), 0o600))
	return home
}

func shippedClaude(t *testing.T) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "..", "engine", "agents", "internal",
		"protocol", "internal", "descriptor", "descriptors-v3", "claude.yaml"))
	require.NoError(t, err)
	return raw
}

func TestDescriptorReports_SurfacesAHomeFailure(t *testing.T) {
	t.Parallel()
	table := provider.New(provider.Deps{
		Agents: engineagents.New(), Home: func() (string, error) { return "", assert.AnError },
	})

	_, err := table.DescriptorReports(t.Context())
	require.ErrorIs(t, err, assert.AnError)
}

func TestResolveProviders_CarriesTerminalStartHereFromTheDescriptor(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })

	providers, err := table.ResolveProviders(t.Context())

	require.NoError(t, err)
	byID := map[string]domain.AgentProvider{}
	for _, p := range providers {
		byID[p.ID] = p
	}
	require.True(t, byID["claude"].HasTerminal)
	assert.True(t, byID["claude"].TerminalStartHere)
	require.True(t, byID["codex"].HasTerminal)
	assert.True(t, byID["codex"].TerminalStartHere,
		"codex's terminal surface rides the hooks channel: spawnable from birth, no session to resume")
}

func TestResolveProviders_CarriesTheSignInCommandFromTheDescriptor(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })

	providers, err := table.ResolveProviders(t.Context())

	require.NoError(t, err)
	byID := map[string]domain.AgentProvider{}
	for _, p := range providers {
		byID[p.ID] = p
	}
	assert.Equal(t, "claude auth login", byID["claude"].SignIn)
	assert.Equal(t, "codex login", byID["codex"].SignIn)
}

func TestResolveProviders_ClaudeEffortsCarriesTheEmptyModelKey(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })

	providers, err := table.ResolveProviders(t.Context())

	require.NoError(t, err)
	var claude domain.AgentProvider
	for _, p := range providers {
		if p.ID == "claude" {
			claude = p
		}
	}
	require.NotEmpty(t, claude.Models, "claude's manifest-derived catalogue must have resolved")
	require.NotNil(t, claude.Efforts, "a bad '' key must never make the whole map vanish")
	assert.NotEmpty(t, claude.Efforts[""],
		"'' is the union of every model's own levels — see modeldiscovery.effortsOf")
	for _, model := range claude.Models {
		assert.NotEmpty(t, claude.Efforts[model], "every selectable model must answer its levels")
	}
}

func TestResolveProviders_ReportsEachDiscoveredModelsDefaultEffort(t *testing.T) {
	t.Parallel()
	home := t.TempDir()
	cache := filepath.Join(home, "model-discover-cache")
	require.NoError(t, os.MkdirAll(cache, 0o700))
	require.NoError(t, os.WriteFile(filepath.Join(cache, "codex.json"), []byte(`[
		{"ID":"m1","Label":"M1","Efforts":["low","medium","high"],"DefaultEffort":"medium"},
		{"ID":"m2","Label":"M2","Efforts":["low","high"],"DefaultEffort":""}]`), 0o600))

	providers, err := newTableAt(t, home, func(engineagents.Agent) bool { return false }).
		ResolveProviders(t.Context())

	require.NoError(t, err)
	for _, p := range providers {
		if p.ID == "codex" {
			assert.Equal(t, map[string]string{"m1": "medium"}, p.DefaultEfforts,
				"a model stating no default is absent, never guessed")
			return
		}
	}
	t.Fatal("codex not listed")
}

func TestResolveProviders_ConnectedComesFromTheInstallProbe(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(a engineagents.Agent) bool { return a.ID() == "codex" })

	providers, err := table.ResolveProviders(t.Context())

	require.NoError(t, err)
	for _, p := range providers {
		assert.Equal(t, p.ID == "codex", p.Connected, "provider %s", p.ID)
	}
}

func TestDefaultProviderID_IsTheFirstEnabledInstalledProviderInTheUsersOrder(t *testing.T) {
	t.Parallel()
	all := func(engineagents.Agent) bool { return true }

	ordered := newTable(t, all)
	_, err := ordered.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "codex", Priority: 0}, {ProviderID: "claude", Priority: 1},
	})
	require.NoError(t, err)
	disabled := newTable(t, all)
	_, err = disabled.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "claude", Priority: 0, Disabled: true}, {ProviderID: "codex", Priority: 1},
	})
	require.NoError(t, err)

	for name, tc := range map[string]struct {
		table *provider.Providers
		want  string
	}{
		"the order's first":        {ordered, "codex"},
		"skips a disabled one":     {disabled, "codex"},
		"skips an uninstalled one": {newTable(t, func(a engineagents.Agent) bool { return a.ID() == "codex" }), "codex"},
		"none installed":           {newTable(t, func(engineagents.Agent) bool { return false }), ""},
	} {
		got, err := tc.table.DefaultProviderID(t.Context())
		require.NoError(t, err, name)
		assert.Equal(t, tc.want, got, name)
	}
}

func TestNew_NilInstallProbeDefaultsToTheRealOne(t *testing.T) {
	t.Parallel()

	table := newTable(t, nil)

	_, err := table.ResolveProviders(t.Context())

	require.NoError(t, err)
}

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

	require.ErrorIs(t, err, problem.ErrInvalidArgument)
}

func TestRequireProviderEnabled_RefusesADisabledProvider(t *testing.T) {
	t.Parallel()

	table := newTable(t, func(engineagents.Agent) bool { return false })
	_, err := table.ReplaceProviderPreferences(t.Context(), []domain.AgentProviderPreference{
		{ProviderID: "claude", Disabled: true},
	})
	require.NoError(t, err)

	err = table.RequireProviderEnabled(t.Context(), "claude")

	require.ErrorIs(t, err, provider.ErrProviderDisabled)
	require.ErrorIs(t, err, problem.ErrInvalidArgument, "handlers answer 400 off this class")
	assert.NoError(t, table.RequireProviderEnabled(t.Context(), "codex"))
	assert.NoError(t, table.RequireProviderEnabled(t.Context(), "never-preferenced"),
		"a provider with no row is enabled")
}

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

func TestDispatchMCP_RefusesAnUnconfiguredToolSurface(t *testing.T) {
	t.Parallel()

	table := provider.New(provider.Deps{})

	_, send, err := table.DispatchMCP(t.Context(), "runner-1", "token",
		[]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))

	require.Error(t, err)
	assert.False(t, send)
}
