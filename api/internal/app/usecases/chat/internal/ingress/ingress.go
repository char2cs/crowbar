package ingress

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	ledgerstore "github.com/char2cs/crowbar/api/internal/adapter/store/ledger"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/seam"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

var (
	ErrUnknownRunner = fmt.Errorf("ingress: unknown runner: %w", problem.ErrNotFound)
	ErrUnmappable    = fmt.Errorf("ingress: unmappable hook: %w", problem.ErrUnprocessable)
)

type Reply struct {
	Body []byte
	Ask  string
}

type Runners interface {
	RunnerByID(runnerID string) (engineagents.Runner, bool)
	RecordedRunner(ctx context.Context, runnerID string) (engineagents.Runner, bool, error)
}

type Descriptors interface {
	Descriptor(ctx context.Context, runner engineagents.Runner) (engineagents.Agent, error)
}

type Turns interface {
	RecordTelemetry(
		ctx context.Context, runner engineagents.Runner, agent engineagents.Agent, raw []byte,
		channel engineagents.Channel,
	) error
	Mark(ctx context.Context, runner engineagents.Runner, row ledger.Row) error
	Dispatch(
		ctx context.Context, runner engineagents.Runner, agent engineagents.Agent,
		ev engineagents.CanonicalEvent, raw []byte, channel engineagents.Channel,
	) (reply []byte, ask string, err error)
}

type Ledger interface {
	DeadLetter(ctx context.Context, d ledgerstore.DeadLetter) error
	DeadLetterCount(ctx context.Context, reasons ...string) (int, error)
}

type Deps struct {
	Runners     Runners
	Descriptors Descriptors
	Turns       Turns
	Ledger      Ledger
	Locks       *Locks
}

type Ingress struct {
	runners     Runners
	descriptors Descriptors
	turns       Turns
	ledger      Ledger
	locks       *Locks
}

func New(d Deps) *Ingress {
	return &Ingress{
		runners: d.Runners, descriptors: d.Descriptors, turns: d.Turns, ledger: d.Ledger, locks: d.Locks,
	}
}

func (i *Ingress) Hook(
	ctx context.Context,
	runnerID, provider, event string,
	raw []byte,
	channel engineagents.Channel,
) (Reply, error) {
	h := hook{runnerID: runnerID, provider: provider, event: event, raw: raw}
	runner, ok, err := i.resolve(ctx, runnerID)
	if err != nil {
		return Reply{}, i.refuse(ctx, h, "", reasonUnknownRunner, err)
	}
	if !ok {
		slog.DebugContext(ctx, "ingress: drop a hook of an erased chat's runner", "runner_id", runnerID, "event", event)
		return Reply{}, ErrUnknownRunner
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
			return i.turns.RecordTelemetry(ctx, runner, agent, raw, channel)
		})
	}
	ev, err := agent.ParseHook(event, raw, channel)
	switch {
	case errors.Is(err, engineagents.ErrHookVariantMismatch):
		return Reply{}, nil
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
	case errors.Is(err, problem.ErrUnprocessable):
		return i.refuse(ctx, h, chatID, reasonUnmappable, fmt.Errorf("%w: %s: %w", ErrUnmappable, h.event, err))
	case errors.Is(err, problem.ErrNotFound):
		return i.refuse(ctx, h, chatID, reasonUnknownChat, fmt.Errorf("ingress: %s: %w", h.event, err))
	}
	return i.refuse(ctx, h, chatID, reasonWriteFailed, fmt.Errorf("ingress: %s: %w", h.event, err))
}
