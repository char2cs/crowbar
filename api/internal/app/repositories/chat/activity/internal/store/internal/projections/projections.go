package projections

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/char2cs/crowbar/api/internal/app/repositories/chat/activity/internal/store/internal/storage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

type Projector struct {
	store *storage.Store
}

func New(store *storage.Store) *Projector {
	return &Projector{store: store}
}

func (p *Projector) Apply(ctx context.Context, activity domain.ChatActivity) error {
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

	if err := p.abandonRunningSubagents(ctx, delta); err != nil {
		return err
	}

	if err := p.store.ResolveOpenInterruptions(ctx, delta.Turn.ChatID, delta.Turn.EndedAt); err != nil {
		return fmt.Errorf("agentactivity projection: resolve open interruptions: %w", err)
	}

	if err := p.store.ResolveOpenChoices(ctx, delta.Turn.ChatID, delta.Turn.EndedAt); err != nil {
		return fmt.Errorf("agentactivity projection: resolve open choices: %w", err)
	}
	return nil
}

// abandonRunningSubagents is gated on delta.Abandoned, unlike
// AbandonRunningTools above: a subagent is deliberately allowed to keep
// running past its OWN turn's ordinary close (codex hands off and ends its
// turn right there — see turn.go's restateAsyncWork doc, proven by
// TestRegression_CodexTurnStopWithOpenSubagent_KeepsChatWorking). Only a
// delta that gives up on the turn's work entirely — the CLI process is gone,
// or it produced nothing — may treat a still-open subagent as abandoned
// rather than legitimately in flight.
func (p *Projector) abandonRunningSubagents(ctx context.Context, delta *domain.ActivityDelta) error {
	if !delta.Abandoned {
		return nil
	}
	closed, err := p.store.AbandonRunningSubagents(ctx, delta.Turn.ChatID, delta.Turn.EndedAt)
	if err != nil {
		return fmt.Errorf("agentactivity projection: abandon running subagents: %w", err)
	}
	if closed == 0 {
		return nil
	}
	// A leak: a subagent still open when the CHAT is given up on entirely means
	// its subagent_post never arrived — crashed, killed mid-run, or dropped —
	// and without this call OpenWork would have seen it as open forever, chat-
	// wide, well past this turn. See AbandonRunningSubagents' own doc.
	slog.WarnContext(ctx, "agentactivity projection: closed subagent(s) their own turn never heard finish",
		"chat_id", delta.Turn.ChatID, "turn_id", delta.Turn.ID, "count", closed)
	return nil
}

func (p *Projector) Forget(ctx context.Context, chatID string) error {
	return p.store.DeleteChat(ctx, chatID)
}
