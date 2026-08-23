package projections

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/storage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type Projector struct {
	store *storage.Store
}

func New(store *storage.Store) *Projector {
	return &Projector{store: store}
}

func (p *Projector) Apply(ctx context.Context, activity domain.AgentActivity) error {
	delta := activity.Last
	if delta == nil {
		return nil
	}
	switch delta.Kind {
	case domain.DeltaTurn:
		return p.applyTurn(ctx, delta)
	case domain.DeltaTool:
		if delta.Tool == nil {
			return nil
		}
		return p.applyTool(ctx, delta)
	case domain.DeltaSubagent:
		if delta.Subagent == nil {
			return nil
		}
		return p.store.SaveSubagent(ctx, *delta.Subagent)
	case domain.DeltaInterruption:
		if delta.Interruption == nil {
			return nil
		}
		return p.store.SaveInterruption(ctx, *delta.Interruption)
	case domain.DeltaChoice:
		if delta.Choice == nil {
			return nil
		}
		return p.store.SaveChoice(ctx, *delta.Choice)
	default:
		return nil
	}
}

func (p *Projector) applyTool(ctx context.Context, delta *domain.ActivityDelta) error {
	call := *delta.Tool
	if err := p.store.SaveToolCall(ctx, call); err != nil {
		return err
	}
	if delta.Phase != domain.DeltaClose {
		return nil
	}
	if err := p.store.ResolveChoicesForTool(
		ctx, call.ChatID, call.ID, call.Name, call.EndedAt,
	); err != nil {
		return fmt.Errorf("agentactivity projection: resolve choices for tool: %w", err)
	}
	return nil
}

func (p *Projector) applyTurn(ctx context.Context, delta *domain.ActivityDelta) error {
	if delta.Turn == nil {
		return nil
	}

	if delta.Phase != domain.DeltaClose {
		return nil
	}

	if delta.Turn.Text != "" {
		if err := p.store.SaveTurn(ctx, *delta.Turn); err != nil {
			return err
		}
	}

	if delta.SupersededTurnID != "" && delta.SupersededTurnID != delta.Turn.ID {
		if err := p.store.RepointActivity(
			ctx, delta.Turn.ChatID, delta.SupersededTurnID, delta.Turn.ID,
		); err != nil {
			return fmt.Errorf("agentactivity projection: repoint activity: %w", err)
		}
	}

	if err := p.store.AbandonRunningTools(ctx, delta.Turn.ChatID, delta.Turn.EndedAt); err != nil {
		return fmt.Errorf("agentactivity projection: abandon running tools: %w", err)
	}

	if err := p.store.ResolveOpenInterruptions(ctx, delta.Turn.ChatID, delta.Turn.EndedAt); err != nil {
		return fmt.Errorf("agentactivity projection: resolve open interruptions: %w", err)
	}

	if err := p.store.ResolveOpenChoices(ctx, delta.Turn.ChatID, delta.Turn.EndedAt); err != nil {
		return fmt.Errorf("agentactivity projection: resolve open choices: %w", err)
	}
	return nil
}

func (p *Projector) Forget(ctx context.Context, chatID string) error {
	return p.store.DeleteChat(ctx, chatID)
}
