// Package provider owns the global provider table — which vendor CLIs this
// machine offers, in what order, and with which of them allowed to call back
// into Crowbar — and the MCP dispatch those callbacks arrive on.
//
// Everything here is per user/machine and never per workspace: the settings PUT
// that rewrites the table has no workspace to resolve one from, so nothing in
// this package may depend on there being one.
package provider

import (
	"context"
	"fmt"
	"sort"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/tools"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// ErrProviderDisabled is returned when a request names a provider the user has
// switched off. It is an invalid argument rather than a not-found: the provider
// exists, and the user chose not to use it.
var ErrProviderDisabled = fmt.Errorf("agent: provider disabled: %w", apperr.ErrInvalidArgument)

// Providers is the provider table.
type Providers struct {
	agents engineagents.Agents
	// home resolves crowbar home for the descriptor catalog. It is the app-config
	// resolver, NOT a wsId lookup: providers are global, so provider resolution
	// must not depend on any workspace.
	home func() (string, error)
	// installed is the install probe (defaults to Agent.Installed); injectable so
	// provider-resolution tests never depend on the host having claude/codex.
	installed func(a engineagents.Agent) bool
	// prefs is the global priority+enabled table read by ResolveProviders and
	// rewritten by ReplaceProviderPreferences. It is keyed by provider id; a
	// provider with no row is enabled and ordered after every preferenced one by
	// descriptor id.
	prefs store.Store[domain.AgentProviderPreference, string]
	// minter issues the per-runner token an MCP call is authenticated by. It is
	// the SAME instance the spawn path hands a runner its token from: a runner's
	// token must be minted by the same secret DispatchMCP verifies against.
	minter *agenttools.TokenMinter
	// tools is the agent-facing capability surface DispatchMCP builds a per-call
	// ToolSet from. Its self-ports are wired by the caller before this is built;
	// the one dependency a caller can get wrong is the Resolver — and DispatchMCP
	// refuses to serve without it rather than quietly advertising an empty tool
	// list.
	tools agenttools.Deps
}

// Deps is everything the provider table is built over.
type Deps struct {
	Agents engineagents.Agents
	Home   func() (string, error)
	// Installed is the install probe. Nil defaults to the real one, so only a test
	// has to think about it.
	Installed func(a engineagents.Agent) bool
	Prefs     store.Store[domain.AgentProviderPreference, string]
	Minter    *agenttools.TokenMinter
	Tools     agenttools.Deps
}

// New builds the provider table.
func New(d Deps) *Providers {
	installed := d.Installed
	if installed == nil {
		installed = func(a engineagents.Agent) bool { return a.Installed() }
	}
	return &Providers{
		agents:    d.Agents,
		home:      d.Home,
		installed: installed,
		prefs:     d.Prefs,
		minter:    d.Minter,
		tools:     d.Tools,
	}
}

func (p *Providers) RequireProviderEnabled(
	ctx context.Context,
	providerID string,
) error {
	pref, err := p.prefs.FindByKey(ctx, providerID)
	if err != nil {
		return fmt.Errorf("agent: provider preference %q: %w", providerID, err)
	}
	if pref != nil && pref.Disabled {
		return fmt.Errorf("%w (%q)", ErrProviderDisabled, providerID)
	}
	return nil
}

func (p *Providers) ProviderMCPEnabled(
	ctx context.Context,
	providerID string,
) (bool, error) {
	pref, err := p.prefs.FindByKey(ctx, providerID)
	if err != nil {
		return false, fmt.Errorf("agent: provider mcp preference %q: %w", providerID, err)
	}
	return pref == nil || !pref.MCPDisabled, nil
}

func (p *Providers) ResolveProviders(
	ctx context.Context,
) ([]domain.AgentProvider, error) {
	home, err := p.home()
	if err != nil {
		return nil, fmt.Errorf("agent: resolve providers: home: %w", err)
	}
	descs, err := p.agents.List(ctx, home)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve providers: descriptors: %w", err)
	}
	prefs, err := p.prefs.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve providers: preferences: %w", err)
	}
	byID := make(map[string]domain.AgentProviderPreference, len(prefs))
	for _, p := range prefs {
		byID[p.ProviderID] = p
	}

	out := make([]domain.AgentProvider, 0, len(descs))
	for _, d := range descs {
		pref := byID[d.ID()]
		display := d.Display()
		caps := d.Capabilities()
		out = append(out, domain.AgentProvider{
			ID:               d.ID(),
			DisplayName:      display.Name,
			Icon:             display.Icon,
			Connected:        p.installed(d),
			Enabled:          !pref.Disabled,
			MCPEnabled:       !pref.MCPDisabled,
			Compaction:       caps.Compaction,
			ModelSelect:      caps.ModelSelect,
			EffortSelect:     caps.EffortSelect,
			Hotswap:          caps.Hotswap,
			HasTerminal:      caps.HasTerminal,
			Models:           d.Models(),
			Efforts:          resolveEfforts(d),
			PermissionLevels: d.PermissionLevels(),
		})
	}
	sort.SliceStable(out, func(i, j int) bool {
		pi, oki := byID[out[i].ID]
		pj, okj := byID[out[j].ID]
		if oki != okj {
			return oki // a preferenced provider sorts before an unpreferenced one
		}
		if oki && pi.Priority != pj.Priority {
			return pi.Priority < pj.Priority
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (p *Providers) ReplaceProviderPreferences(
	ctx context.Context,
	prefs []domain.AgentProviderPreference,
) ([]domain.AgentProvider, error) {
	home, err := p.home()
	if err != nil {
		return nil, fmt.Errorf("agent: replace provider preferences: home: %w", err)
	}
	descs, err := p.agents.List(ctx, home)
	if err != nil {
		return nil, fmt.Errorf("agent: replace provider preferences: descriptors: %w", err)
	}
	known := make(map[string]struct{}, len(descs))
	for _, d := range descs {
		known[d.ID()] = struct{}{}
	}
	submitted := make(map[string]struct{}, len(prefs))
	for _, pref := range prefs {
		if _, ok := known[pref.ProviderID]; !ok {
			return nil, fmt.Errorf("agent: replace provider preferences: unknown provider %q: %w",
				pref.ProviderID, apperr.ErrInvalidArgument)
		}
		submitted[pref.ProviderID] = struct{}{}
	}

	// Delete stored rows the submission omits FIRST, so a provider dropped from the
	// set reverts to default rather than lingering with a stale priority.
	existing, err := p.prefs.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: replace provider preferences: list existing: %w", err)
	}
	for _, e := range existing {
		if _, ok := submitted[e.ProviderID]; ok {
			continue
		}
		if err := p.prefs.Delete(ctx, e.ProviderID); err != nil {
			return nil, fmt.Errorf("agent: replace provider preferences: delete %q: %w", e.ProviderID, err)
		}
	}
	for _, pref := range prefs {
		if err := p.prefs.Save(ctx, pref); err != nil {
			return nil, fmt.Errorf("agent: replace provider preferences: save %q: %w", pref.ProviderID, err)
		}
	}
	return p.ResolveProviders(ctx)
}

func resolveEfforts(
	agent engineagents.Agent,
) map[string][]string {
	if !agent.Capabilities().EffortSelect {
		return nil
	}
	out := map[string][]string{}
	for _, model := range append([]string{""}, agent.Models()...) {
		if levels := agent.Efforts(model); len(levels) > 0 {
			out[model] = levels
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
