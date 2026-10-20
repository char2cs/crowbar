package ingress

import (
	"context"
	"fmt"
	"log/slog"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// Why a hook was dead-lettered.
const (
	reasonUnknownRunner = "unknown_runner"
	reasonUnknownChat   = "unknown_chat"
	reasonForeign       = "foreign_conversation"
	reasonUnmappable    = "unmappable"
	reasonUnrecorded    = "unrecorded"
)

// hook is one delivery as its sender made it.
type hook struct {
	runnerID, provider, event string
	raw                       []byte
}

// deadLetter keeps h, which the ingress could not place on chatID. The write
// outlives the request, and its failure is only logged: the sender's answer
// does not depend on it.
func (i *Ingress) deadLetter(ctx context.Context, h hook, chatID, reason string) {
	err := i.ledger.DeadLetter(context.WithoutCancel(ctx), ledgerstore.DeadLetter{
		ChatID: chatID, RunnerID: h.runnerID, Provider: h.provider, Event: h.event,
		Reason: reason, Payload: h.raw,
	})
	if err != nil {
		slog.ErrorContext(ctx, "ingress: dead letter", "reason", reason, "runner_id", h.runnerID,
			"event", h.event, "err", err)
	}
}

// unmappable puts a marker on chatID's transcript, so the gap shows where it
// happened, then dead-letters h.
func (i *Ingress) unmappable(ctx context.Context, h hook, chatID string, cause error) error {
	row, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{
		Kind: ledger.MarkerDeadLetter, Detail: h.event + ": " + cause.Error(),
	})
	if err == nil {
		unlock := i.locks.Lock(chatID)
		_, _, err = i.ledger.Append(context.WithoutCancel(ctx), chatID, row)
		unlock()
	}
	if err != nil {
		slog.ErrorContext(ctx, "ingress: mark an unmappable hook", "chat_id", chatID, "event", h.event, "err", err)
	}
	i.deadLetter(ctx, h, chatID, reasonUnmappable)
	return fmt.Errorf("%w: %s: %w", ErrUnmappable, h.event, cause)
}
