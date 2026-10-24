// Package ingress places every hook a CLI sends, over any channel, on the chat
// it belongs to. It resolves the runner through the live table, parses through
// the runner's descriptor and records under the chat's lock, answering only once
// the rows are in. What it cannot place is refused and dead-lettered.
package ingress

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"log/slog"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

var (
	// ErrUnknownRunner is a hook from a runner no live table or chat holds.
	ErrUnknownRunner = fmt.Errorf("ingress: unknown runner: %w", apperr.ErrNotFound)
	// ErrUnmappable is a hook its runner's descriptor cannot map.
	ErrUnmappable = fmt.Errorf("ingress: unmappable hook: %w", apperr.ErrUnprocessable)
)

// Reply is what the sender of a placed hook is answered with.
type Reply struct {
	// Body is the stdout the CLI receives (a steered prompt); nil is an empty answer.
	Body []byte
	// Ask, when set, is the ask the sender's answer is held on.
	Ask string
}

// Runners resolves the runner a hook names: in the live table, else as its
// chat's ledger started it.
type Runners interface {
	RunnerByID(runnerID string) (engineagents.Runner, bool)
	RecordedRunner(ctx context.Context, runnerID string) (engineagents.Runner, bool, error)
}

// Descriptors resolves the descriptor a runner's hooks are parsed with.
type Descriptors interface {
	Descriptor(ctx context.Context, runner engineagents.Runner) (engineagents.Agent, error)
}

// Turns records one hook on its runner's chat. Dispatch returns nil once the
// hook's rows are committed, or for a kind it filters by design; a write that
// failed is its error, and a hook it keeps nothing of is seam.ErrHookNotRecorded.
type Turns interface {
	RecordTelemetry(ctx context.Context, runner engineagents.Runner, agent engineagents.Agent, raw []byte) error
	// Mark records row, the trace of a hook nothing else is kept of, on runner's chat.
	Mark(ctx context.Context, runner engineagents.Runner, row ledger.Row) error
	Dispatch(
		ctx context.Context, runner engineagents.Runner, agent engineagents.Agent,
		ev engineagents.CanonicalEvent, raw []byte, channel engineagents.Channel,
	) (reply []byte, ask string, err error)
}

// Ledger keeps every dead letter.
type Ledger interface {
	DeadLetter(ctx context.Context, d ledgerstore.DeadLetter) error
	DeadLetterCount(ctx context.Context, reasons ...string) (int, error)
}

// Deps is everything the ingress is built over.
type Deps struct {
	Runners     Runners
	Descriptors Descriptors
	Turns       Turns
	Ledger      Ledger
	// Locks is shared with the user-initiated spawns: one lock orders both.
	Locks *Locks
}

// Ingress is the one door every hook comes in through.
type Ingress struct {
	runners     Runners
	descriptors Descriptors
	turns       Turns
	ledger      Ledger
	locks       *Locks
}

// New builds the ingress.
func New(d Deps) *Ingress {
	return &Ingress{
		runners: d.Runners, descriptors: d.Descriptors, turns: d.Turns, ledger: d.Ledger, locks: d.Locks,
	}
}

// Hook places one hook from runnerID. It returns once the hook's rows are
// committed; an ask's answer is held by the caller, never under the chat's lock.
func (i *Ingress) Hook(
	ctx context.Context,
	runnerID, provider, event string,
	raw []byte,
	channel engineagents.Channel,
) (Reply, error) {
	h := hook{runnerID: runnerID, provider: provider, event: event, raw: raw}
	runner, ok, err := i.resolve(ctx, runnerID)
	if err != nil || !ok {
		return Reply{}, i.refuse(ctx, h, "", reasonUnknownRunner, cmp.Or(err, ErrUnknownRunner))
	}
	chatID := runner.CurrentChatID
	if provider != "" && provider != runner.ProviderID {
		slog.WarnContext(ctx, "ingress: hook names another provider than its runner",
			"hook_provider", provider, "runner_provider", runner.ProviderID, "runner_id", runnerID)
	}
	agent, err := i.descriptors.Descriptor(ctx, runner)
	if err != nil {
		return Reply{}, i.refuse(ctx, h, chatID, reasonNoDescriptor, fmt.Errorf("ingress: resolve descriptor: %w", err))
	}
	if event == engineagents.HookTelemetry {
		return Reply{}, i.record(ctx, h, runner, func() error {
			return i.turns.RecordTelemetry(ctx, runner, agent, raw)
		})
	}
	ev, err := agent.ParseHook(event, raw, channel)
	switch {
	case errors.Is(err, engineagents.ErrHookVariantMismatch):
		return Reply{}, nil // a declared filter: another variant of the same wire hook
	case errors.Is(err, engineagents.ErrForeignConversation):
		return Reply{}, i.deadLetter(ctx, h, chatID, reasonForeign)
	case err != nil:
		return Reply{}, i.unmappable(ctx, h, runner, err)
	}
	var reply Reply
	err = i.record(ctx, h, runner, func() error {
		var err error
		reply.Body, reply.Ask, err = i.turns.Dispatch(ctx, runner, agent, ev, raw, channel)
		return err
	})
	if err != nil {
		return Reply{}, err
	}
	return reply, nil
}

// resolve is the runner a hook names: as the live table holds it, else as its
// chat's ledger started it, for a hook its CLI sent before it died.
func (i *Ingress) resolve(ctx context.Context, runnerID string) (engineagents.Runner, bool, error) {
	if runner, ok := i.runners.RunnerByID(runnerID); ok {
		return runner, true, nil
	}
	runner, ok, err := i.runners.RecordedRunner(ctx, runnerID)
	if err != nil {
		return engineagents.Runner{}, false, fmt.Errorf("ingress: resolve runner %s: %w", runnerID, err)
	}
	return runner, ok, nil
}

// record runs write under the lock of runner's chat. A runner that exited
// meanwhile still takes its hook; a chat purged meanwhile refuses it. What
// write could not record is dead-lettered.
func (i *Ingress) record(ctx context.Context, h hook, runner engineagents.Runner, write func() error) error {
	chatID := runner.CurrentChatID
	unlock, err := i.locks.Lock(ctx, chatID)
	if err != nil {
		return i.refuse(ctx, h, chatID, reasonSenderGone, fmt.Errorf("ingress: %s: %w", h.event, err))
	}
	defer unlock()
	err = write()
	switch {
	case err == nil:
		return nil
	case errors.Is(err, engineagents.ErrForeignConversation):
		return i.deadLetter(ctx, h, chatID, reasonForeign)
	case errors.Is(err, seam.ErrHookNotRecorded):
		return i.deadLetter(ctx, h, chatID, reasonNotRecorded)
	case errors.Is(err, apperr.ErrUnprocessable):
		return i.refuse(ctx, h, chatID, reasonUnmappable, fmt.Errorf("%w: %s: %w", ErrUnmappable, h.event, err))
	case errors.Is(err, apperr.ErrNotFound):
		return i.refuse(ctx, h, chatID, reasonUnknownChat, fmt.Errorf("ingress: %s: %w", h.event, err))
	}
	return i.refuse(ctx, h, chatID, reasonWriteFailed, fmt.Errorf("ingress: %s: %w", h.event, err))
}
