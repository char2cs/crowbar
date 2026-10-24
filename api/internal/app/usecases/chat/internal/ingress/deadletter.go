package ingress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Why a hook was dead-lettered.
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

// refusals are the reasons a hook was refused to its sender; a hook kept for any
// other was acknowledged, as its channel expects.
var refusals = []string{
	reasonUnknownRunner, reasonUnknownChat, reasonUnmappable, reasonNoDescriptor, reasonWriteFailed, reasonSenderGone,
}

// Refusals is how many hooks were refused to their senders and kept.
func (i *Ingress) Refusals(ctx context.Context) (int, error) {
	n, err := i.ledger.DeadLetterCount(ctx, refusals...)
	if err != nil {
		return 0, fmt.Errorf("ingress: refusals: %w", err)
	}
	return n, nil
}

// hook is one delivery as its sender made it.
type hook struct {
	runnerID, provider, event string
	raw                       []byte
}

// deadLetter keeps h, which the ingress could not place on chatID. The write
// outlives the request. A hook acknowledged without rows is answered with its
// error, so a 2xx always leaves rows or a dead letter behind.
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

// refuse dead-letters h and answers its sender with refused. A dead letter that
// fails is only logged: the sender is refused either way.
func (i *Ingress) refuse(ctx context.Context, h hook, chatID, reason string, refused error) error {
	if err := i.deadLetter(ctx, h, chatID, reason); err != nil {
		slog.ErrorContext(ctx, "ingress: refuse a hook", "runner_id", h.runnerID, "event", h.event, "err", err)
	}
	return refused
}

// unmappable marks the transcript of runner's chat where h was lost and
// refuses h. A chat gone meanwhile is marked nowhere.
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
		if err != nil && !errors.Is(err, apperr.ErrNotFound) {
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
