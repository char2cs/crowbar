package ingress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

const (
	reasonUnknownRunner = "unknown_runner"
	reasonUnknownChat   = "unknown_chat"
	reasonForeign       = "foreign_conversation"
	reasonNotRecorded   = "not_recorded"
	reasonUnmappable    = "unmappable"
	reasonNoDescriptor  = "no_descriptor"
	reasonWriteFailed   = "write_failed"
	reasonSenderGone    = "sender_gone"
)

var refusals = []string{
	reasonUnknownRunner, reasonUnknownChat, reasonUnmappable, reasonNoDescriptor, reasonWriteFailed,
}

func (i *Ingress) Refusals(ctx context.Context) (int, error) {
	n, err := i.ledger.DeadLetterCount(ctx, refusals...)
	if err != nil {
		return 0, fmt.Errorf("ingress: refusals: %w", err)
	}
	return n, nil
}

type hook struct {
	runnerID, provider, event string
	raw                       []byte
}

func (i *Ingress) deadLetter(ctx context.Context, h hook, chatID, reason string) error {
	err := i.ledger.DeadLetter(context.WithoutCancel(ctx), ledgerstore.DeadLetter{
		ChatID: chatID, RunnerID: h.runnerID, Provider: h.provider, Event: h.event,
		Reason: reason, Payload: h.raw,
	})
	if err != nil {
		return fmt.Errorf("ingress: dead letter %s: %w", reason, err)
	}
	return nil
}

func (i *Ingress) refuse(ctx context.Context, h hook, chatID, reason string, refused error) error {
	if ctx.Err() != nil {
		reason = reasonSenderGone
	}
	if err := i.deadLetter(ctx, h, chatID, reason); err != nil {
		slog.ErrorContext(ctx, "ingress: refuse a hook", "runner_id", h.runnerID, "event", h.event, "err", err)
	}
	return refused
}

func (i *Ingress) unmappable(ctx context.Context, h hook, runner engineagents.Runner, cause error) error {
	chatID := runner.CurrentChatID
	refused := fmt.Errorf("%w: %s: %w", ErrUnmappable, h.event, cause)
	row, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{
		Kind: ledger.MarkerDeadLetter, Detail: h.event + ": " + cause.Error(),
	})
	if err != nil {
		return i.refuse(ctx, h, chatID, reasonUnmappable, refused)
	}
	err = i.record(ctx, h, runner, func() error {
		err := i.turns.Mark(context.WithoutCancel(ctx), runner, row)
		if err != nil && !errors.Is(err, problem.ErrNotFound) {
			slog.ErrorContext(ctx, "ingress: mark an unmappable hook", "chat_id", chatID, "event", h.event, "err", err)
			return nil
		}
		return err
	})
	if err != nil {
		return err
	}
	return i.refuse(ctx, h, chatID, reasonUnmappable, refused)
}
