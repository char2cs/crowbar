// Package ingress places every hook a CLI sends, over any channel, on the chat
// it belongs to. It resolves the runner through the live table, parses through
// the runner's descriptor and records under the chat's lock, answering only once
// the rows are in. What it cannot place is refused and dead-lettered.
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

var (
	// ErrUnknownRunner is a hook from a runner the live table does not hold.
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

// Runners is the live table a hook names its runner in.
type Runners interface {
	RunnerByID(runnerID string) (engineagents.Runner, bool)
}

// Descriptors resolves the descriptor a runner's hooks are parsed with.
type Descriptors interface {
	Descriptor(ctx context.Context, runner engineagents.Runner) (engineagents.Agent, error)
}

// Turns records one hook on its runner's chat. Dispatch returns once the
// hook's rows are committed, with the stdout and the ask its sender is owed.
type Turns interface {
	RecordTelemetry(ctx context.Context, runner engineagents.Runner, agent engineagents.Agent, raw []byte) error
	Dispatch(
		ctx context.Context, runner engineagents.Runner, agent engineagents.Agent,
		ev engineagents.CanonicalEvent, raw []byte, channel engineagents.Channel,
	) (reply []byte, ask string, err error)
}

// Ledger takes an unmappable hook's marker and every dead letter.
type Ledger interface {
	Append(ctx context.Context, chatID string, rows ...ledger.Row) ([]ledger.Row, ledger.State, error)
	DeadLetter(ctx context.Context, d ledgerstore.DeadLetter) error
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

// Lock serialises one chat's hooks and its user-initiated spawns.
func (i *Ingress) Lock(chatID string) (unlock func()) { return i.locks.Lock(chatID) }

// Hook places one hook from runnerID. It returns once the hook's rows are
// committed; an ask's answer is held by the caller, never under the chat's lock.
func (i *Ingress) Hook(
	ctx context.Context,
	runnerID, provider, event string,
	raw []byte,
	channel engineagents.Channel,
) (Reply, error) {
	h := hook{runnerID: runnerID, provider: provider, event: event, raw: raw}
	runner, ok := i.runners.RunnerByID(runnerID)
	if !ok {
		i.deadLetter(ctx, h, "", reasonUnknownRunner)
		return Reply{}, ErrUnknownRunner
	}
	chatID := runner.CurrentChatID
	if provider != "" && provider != runner.ProviderID {
		slog.WarnContext(ctx, "ingress: hook names another provider than its runner",
			"hook_provider", provider, "runner_provider", runner.ProviderID, "runner_id", runnerID)
	}
	agent, err := i.descriptors.Descriptor(ctx, runner)
	if err != nil {
		i.deadLetter(ctx, h, chatID, reasonUnrecorded)
		return Reply{}, fmt.Errorf("ingress: resolve descriptor: %w", err)
	}
	if event == engineagents.HookTelemetry {
		return Reply{}, i.record(ctx, h, chatID, func(r engineagents.Runner) error {
			return i.turns.RecordTelemetry(ctx, r, agent, raw)
		})
	}
	ev, err := agent.ParseHook(event, raw, channel)
	switch {
	case errors.Is(err, engineagents.ErrHookVariantMismatch):
		return Reply{}, nil // a declared filter: another variant of the same wire hook
	case errors.Is(err, engineagents.ErrForeignConversation):
		i.deadLetter(ctx, h, chatID, reasonForeign)
		return Reply{}, nil
	case err != nil:
		return Reply{}, i.unmappable(ctx, h, chatID, err)
	}
	var reply Reply
	err = i.record(ctx, h, chatID, func(r engineagents.Runner) error {
		var err error
		reply.Body, reply.Ask, err = i.turns.Dispatch(ctx, r, agent, ev, raw, channel)
		return err
	})
	if err != nil {
		return Reply{}, err
	}
	return reply, nil
}

// record runs dispatch under chatID's lock, on the runner as the table holds it
// once the lock is taken: a purge takes a chat's runner off the table under it.
func (i *Ingress) record(
	ctx context.Context, h hook, chatID string,
	dispatch func(engineagents.Runner) error,
) error {
	unlock := i.locks.Lock(chatID)
	defer unlock()
	runner, ok := i.runners.RunnerByID(h.runnerID)
	if !ok {
		i.deadLetter(ctx, h, chatID, reasonUnknownRunner)
		return ErrUnknownRunner
	}
	err := dispatch(runner)
	switch {
	case err == nil:
		return nil
	case errors.Is(err, engineagents.ErrForeignConversation):
		i.deadLetter(ctx, h, chatID, reasonForeign)
		return nil
	case errors.Is(err, apperr.ErrNotFound):
		i.deadLetter(ctx, h, chatID, reasonUnknownChat)
	default:
		i.deadLetter(ctx, h, chatID, reasonUnrecorded)
	}
	return fmt.Errorf("ingress: %s: %w", h.event, err)
}
