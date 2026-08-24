package chat

import (
	"context"
	"fmt"
	"sort"

	"github.com/char2cs/crowbar/api/internal/adapter/store"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	agenttools "github.com/char2cs/crowbar/api/internal/app/usecases/chat/tools"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	enginemcp "github.com/char2cs/crowbar/api/internal/engine/mcp"
)

// ProviderUsecase owns the agent PROVIDERS themselves: which ones exist, which
// the user has enabled, and the MCP surface their CLIs call back into.
//
// It is global, never per workspace: a preference row is keyed by provider id
// alone, so nothing here may depend on a workspace being resolvable.
type ProviderUsecase interface {
	// ResolveProviders lists every installed descriptor merged with the user's
	// preference table, ordered by stored priority first and by descriptor id for
	// providers with no row. A provider with no row is enabled.
	ResolveProviders(
		ctx context.Context,
	) ([]domain.AgentProvider, error)

	// ReplaceProviderPreferences overwrites the whole preference table and
	// returns the resolved list the caller should broadcast. A provider the
	// submission omits reverts to its default rather than keeping a stale
	// priority. Returns apperr.ErrInvalidArgument when a row names a provider no
	// descriptor declares.
	ReplaceProviderPreferences(
		ctx context.Context,
		prefs []domain.AgentProviderPreference,
	) ([]domain.AgentProvider, error)

	// DispatchMCP answers one JSON-RPC message from a runner's CLI against a tool
	// set built PER CALL around that runner's credentials, so no handler can be
	// reached without a successful resolve. The bool reports whether the response
	// is to be sent at all (a notification produces none).
	//
	// It refuses to serve at all when the tool surface was never configured,
	// rather than quietly advertising an empty tool list.
	DispatchMCP(
		ctx context.Context,
		runnerID string,
		token string,
		message []byte,
	) ([]byte, bool, error)
}

var _ ProviderUsecase = (*providerUsecase)(nil)

type providerUsecase struct {
	agents engineagents.Agents
	// home resolves crowbar home for the descriptor catalog. It is the app-config
	// resolver, NOT a wsId lookup: providers are global, so provider resolution must
	// not depend on any workspace (the global PUT has no wsId to resolve one from).
	home func() (string, error)
	// installed is the install probe (defaults to Agent.Installed); injectable so
	// provider-resolution tests never depend on the host having claude/codex.
	installed func(a engineagents.Agent) bool
	// providerPrefs is the global (per user/machine) priority+enabled table read by
	// ResolveProviders and rewritten by ReplaceProviderPreferences. It is keyed by
	// provider id; a provider with no row is enabled and ordered after every
	// preferenced one by descriptor id.
	providerPrefs store.Store[domain.AgentProviderPreference, string]
	// minter issues the per-runner token an MCP call is authenticated by. It is
	// the SAME instance the spawn path hands a runner its token from: a runner's
	// token must be minted by the same secret DispatchMCP verifies against.
	minter *agenttools.TokenMinter
	// tools is the agent-facing capability surface DispatchMCP builds a per-call
	// ToolSet from. Its Chats and ChatLogs ports are wired by agent.New; the one
	// dependency a caller can get wrong is the Resolver — and DispatchMCP refuses
	// to serve without it rather than quietly advertising an empty tool list.
	tools agenttools.Deps
}

func newProviderUsecase(
	agents engineagents.Agents,
	home func() (string, error),
	installed func(a engineagents.Agent) bool,
	providerPrefs store.Store[domain.AgentProviderPreference, string],
	minter *agenttools.TokenMinter,
	tools agenttools.Deps,
) *providerUsecase {
	if installed == nil {
		installed = func(a engineagents.Agent) bool { return a.Installed() }
	}
	u := &providerUsecase{
		agents:        agents,
		home:          home,
		installed:     installed,
		providerPrefs: providerPrefs,
		minter:        minter,
		tools:         tools,
	}
	// The per-provider tool switch, wired as a LIVE port rather than read once at
	// spawn: without it a chat spawned with tools on keeps them for the life of
	// its CLI, whatever the user does in Settings afterwards. A nil ToolAccess
	// FAILS OPEN — see agenttools.Deps.refuseDisabledTools.
	u.tools.ToolAccess = u.providerMCPEnabled
	return u
}

func (u *providerUsecase) requireProviderEnabled(
	ctx context.Context,
	providerID string,
) error {
	pref, err := u.providerPrefs.FindByKey(ctx, providerID)
	if err != nil {
		return fmt.Errorf("agent: provider preference %q: %w", providerID, err)
	}
	if pref != nil && pref.Disabled {
		return fmt.Errorf("%w (%q)", ErrProviderDisabled, providerID)
	}
	return nil
}

func (u *providerUsecase) providerMCPEnabled(
	ctx context.Context,
	providerID string,
) (bool, error) {
	pref, err := u.providerPrefs.FindByKey(ctx, providerID)
	if err != nil {
		return false, fmt.Errorf("agent: provider mcp preference %q: %w", providerID, err)
	}
	return pref == nil || !pref.MCPDisabled, nil
}

func (u *providerUsecase) ResolveProviders(
	ctx context.Context,
) ([]domain.AgentProvider, error) {
	home, err := u.home()
	if err != nil {
		return nil, fmt.Errorf("agent: resolve providers: home: %w", err)
	}
	descs, err := u.agents.List(ctx, home)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve providers: descriptors: %w", err)
	}
	prefs, err := u.providerPrefs.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: resolve providers: preferences: %w", err)
	}
	byID := make(map[string]domain.AgentProviderPreference, len(prefs))
	for _, p := range prefs {
		byID[p.ProviderID] = p
	}

	out := make([]domain.AgentProvider, 0, len(descs))
	for _, d := range descs {
		p := byID[d.ID()]
		display := d.Display()
		caps := d.Capabilities()
		out = append(out, domain.AgentProvider{
			ID:           d.ID(),
			DisplayName:  display.Name,
			Icon:         display.Icon,
			Connected:    u.installed(d),
			Enabled:      !p.Disabled,
			MCPEnabled:   !p.MCPDisabled,
			ModelSelect:  caps.ModelSelect,
			EffortSelect: caps.EffortSelect,
			Models:       d.Models(),
			Efforts:      resolveEfforts(d),
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

func (u *providerUsecase) ReplaceProviderPreferences(
	ctx context.Context,
	prefs []domain.AgentProviderPreference,
) ([]domain.AgentProvider, error) {
	home, err := u.home()
	if err != nil {
		return nil, fmt.Errorf("agent: replace provider preferences: home: %w", err)
	}
	descs, err := u.agents.List(ctx, home)
	if err != nil {
		return nil, fmt.Errorf("agent: replace provider preferences: descriptors: %w", err)
	}
	known := make(map[string]struct{}, len(descs))
	for _, d := range descs {
		known[d.ID()] = struct{}{}
	}
	submitted := make(map[string]struct{}, len(prefs))
	for _, p := range prefs {
		if _, ok := known[p.ProviderID]; !ok {
			return nil, fmt.Errorf("agent: replace provider preferences: unknown provider %q: %w",
				p.ProviderID, apperr.ErrInvalidArgument)
		}
		submitted[p.ProviderID] = struct{}{}
	}

	// Delete stored rows the submission omits FIRST, so a provider dropped from the
	// set reverts to default rather than lingering with a stale priority.
	existing, err := u.providerPrefs.FindAll(ctx)
	if err != nil {
		return nil, fmt.Errorf("agent: replace provider preferences: list existing: %w", err)
	}
	for _, e := range existing {
		if _, ok := submitted[e.ProviderID]; ok {
			continue
		}
		if err := u.providerPrefs.Delete(ctx, e.ProviderID); err != nil {
			return nil, fmt.Errorf("agent: replace provider preferences: delete %q: %w", e.ProviderID, err)
		}
	}
	for _, p := range prefs {
		if err := u.providerPrefs.Save(ctx, p); err != nil {
			return nil, fmt.Errorf("agent: replace provider preferences: save %q: %w", p.ProviderID, err)
		}
	}
	return u.ResolveProviders(ctx)
}

func (u *providerUsecase) DispatchMCP(
	ctx context.Context,
	runnerID string,
	token string,
	message []byte,
) ([]byte, bool, error) {
	if u.minter == nil || u.tools.Resolver == nil {
		return nil, false, fmt.Errorf("agent: dispatch mcp: tool surface not configured")
	}
	tools := agenttools.NewToolSet(u.tools, runnerID, token)
	server := enginemcp.NewServer("crowbar", metadata.GetVersion(), tools)
	out, send := server.Handle(ctx, message)
	return out, send, nil
}
