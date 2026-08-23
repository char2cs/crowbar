package agent

import (
	"context"
	"fmt"
	"sort"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

func (u *Usecase) requireProviderEnabled(
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

func (u *Usecase) ResolveProviders(
	ctx context.Context,
) ([]dto.AgentProviderDTO, error) {
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

	out := make([]dto.AgentProviderDTO, 0, len(descs))
	for _, d := range descs {
		p := byID[d.ID()]
		display := d.Display()
		caps := d.Capabilities()
		out = append(out, dto.AgentProviderDTO{
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

func (u *Usecase) ReplaceProviderPreferences(
	ctx context.Context,
	prefs []domain.AgentProviderPreference,
) ([]dto.AgentProviderDTO, error) {
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
