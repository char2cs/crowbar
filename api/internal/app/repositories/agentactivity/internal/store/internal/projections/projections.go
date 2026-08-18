// Package projections keeps the AgentActivity read model in step with the
// aggregate.
//
// It is delta-driven: every handler reads evt.Aggregate.Last — the single item
// the emitting command touched — and upserts exactly that row. It never folds the
// whole aggregate into a blob, because the aggregate deliberately does not hold
// the conversation; only this read model does.
//
// Every write is an upsert keyed by the row's own identity, so replaying an
// already-projected event rewrites identical values. That is what makes a rebuild
// safe without a watermark, and what makes the projection survive being run twice.
package projections

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentactivity/internal/store/internal/storage"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Projector applies one aggregate state to the read model.
type Projector struct {
	store *storage.Store
}

func New(store *storage.Store) *Projector {
	return &Projector{store: store}
}

// Apply writes whatever the emitting command touched.
//
// An event with no delta is not an error: the abandon command emits one when
// there was nothing open, and recording that the reconcile ran is the point.
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

// applyTool writes the call, and — when the call has FINISHED — closes whatever
// prompt was gating it.
//
// The sweep lives here rather than on the delta because a delta carries one item
// and that item is the tool. It is not a shortcut: a permission is answered at the
// PTY, by a human typing into the vendor CLI, and nothing reports that. The gated
// work proceeding is the only evidence there is, and a prompt that waits for
// better evidence stays pinned over a chat that moved on minutes ago.
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
	// An OPEN turn is not yet a turn: it has no text, and a blank row in the
	// conversation reads as the agent having said nothing. That a reply is in
	// flight is already carried by the chat's own working state.
	if delta.Phase != domain.DeltaClose {
		return nil
	}
	// A turn that closed with nothing said is not a message. It happens whenever a
	// CLI is replaced mid-turn — a prompt submission does exactly that — and
	// projecting it would put a blank assistant row in the conversation. Its
	// ACTIVITY is still re-pointed below, so nothing that ran is lost.
	if delta.Turn.Text != "" {
		if err := p.store.SaveTurn(ctx, *delta.Turn); err != nil {
			return err
		}
	}
	// The activity attached itself to the open turn's placeholder identity; the
	// reply landed under the delivery id. Re-pointing is what lets the UI say
	// which tool calls produced which answer.
	if delta.SupersededTurnID != "" && delta.SupersededTurnID != delta.Turn.ID {
		if err := p.store.RepointActivity(
			ctx, delta.Turn.ChatID, delta.SupersededTurnID, delta.Turn.ID,
		); err != nil {
			return fmt.Errorf("agentactivity projection: repoint activity: %w", err)
		}
	}
	// Providers do not guarantee a completion for every invocation. Sweeping at
	// the turn boundary is what stops a tool whose post hook never arrived from
	// rendering as running for the rest of the chat's life.
	if err := p.store.AbandonRunningTools(ctx, delta.Turn.ChatID, delta.Turn.EndedAt); err != nil {
		return fmt.Errorf("agentactivity projection: abandon running tools: %w", err)
	}
	// Nor can an interruption: a notification has no resolving event at all, so an
	// unresolved one would render "the agent needs your attention" forever.
	if err := p.store.ResolveOpenInterruptions(ctx, delta.Turn.ChatID, delta.Turn.EndedAt); err != nil {
		return fmt.Errorf("agentactivity projection: resolve open interruptions: %w", err)
	}
	// Nor can a pending PROMPT. An elicitation has no resolving event on any
	// provider and a permission whose tool never completed has nothing to resolve
	// it, so without this sweep a question the agent stopped asking would stay
	// pinned over the chat for the rest of its life.
	if err := p.store.ResolveOpenChoices(ctx, delta.Turn.ChatID, delta.Turn.EndedAt); err != nil {
		return fmt.Errorf("agentactivity projection: resolve open choices: %w", err)
	}
	return nil
}

// Forget removes a chat's rows when its aggregate is forgotten.
func (p *Projector) Forget(ctx context.Context, chatID string) error {
	return p.store.DeleteChat(ctx, chatID)
}
