package agentrun

import (
	"context"
	"fmt"
	"log/slog"
	"sync"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	agentcmds "github.com/char2cs/crowbar/api/internal/app/repositories/agentrun/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// AgentRun is the repository interface for the AgentRun aggregate.
type AgentRun interface {
	Create(ctx context.Context, taskID string, stateName string, token string) (domain.AgentRun, error)
	Get(ctx context.Context, id string) (domain.AgentRun, error)
	GetByToken(ctx context.Context, token string) (domain.AgentRun, error)
	ListByTask(ctx context.Context, taskID string) ([]domain.AgentRun, error)
	CompleteAgentRun(ctx context.Context, id string, output string) error
	FailAgentRun(ctx context.Context, id string) error
	InterruptAgentRun(ctx context.Context, id string) error
	RecoverOrphanedRuns(ctx context.Context) error
	Subscribe(topic string, fn func(ctx context.Context, evt asynxModels.Event[domain.AgentRun])) (string, error)
}

type agentRunRepo struct {
	ax          asynx.Asynx[domain.AgentRun]
	tokenToID   sync.Map
	mu          sync.Mutex
	ids         []string
	idProvider  func(context.Context) ([]string, error)
	taskPauser  func(context.Context, string) error
}

// Option configures an agentRunRepo.
type Option func(*agentRunRepo)

// WithIDProvider supplies a function that returns all persisted aggregate IDs.
// Used by RecoverOrphanedRuns to find runs that were created in a previous
// process instance.
func WithIDProvider(fn func(context.Context) ([]string, error)) Option {
	return func(r *agentRunRepo) { r.idProvider = fn }
}

// WithTaskPauser supplies a function that pauses a task by ID.
// When set, RecoverOrphanedRuns calls it synchronously after failing each
// orphaned run so the parent task transitions to paused.
func WithTaskPauser(fn func(context.Context, string) error) Option {
	return func(r *agentRunRepo) { r.taskPauser = fn }
}

// New constructs an agentRunRepo wrapping the provided Asynx instance.
func New(
	ax asynx.Asynx[domain.AgentRun],
	opts ...Option,
) AgentRun {
	r := &agentRunRepo{ax: ax}
	for _, o := range opts {
		o(r)
	}
	return r
}

func (r *agentRunRepo) Create(
	ctx context.Context,
	taskID string,
	stateName string,
	token string,
) (domain.AgentRun, error) {
	id := uuid.NewString()
	evt, err := r.ax.SendWait(ctx, agentcmds.CreateAgentRun{
		ID:        id,
		TaskID:    taskID,
		StateName: stateName,
		Token:     token,
	})
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: create: %w", err)
	}

	r.tokenToID.Store(token, id)
	r.mu.Lock()
	r.ids = append(r.ids, id)
	r.mu.Unlock()

	return evt.Aggregate, nil
}

func (r *agentRunRepo) Get(
	ctx context.Context,
	id string,
) (domain.AgentRun, error) {
	run, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.AgentRun{}, fmt.Errorf("agentrun: get %s: %w", id, err)
	}
	return run, nil
}

func (r *agentRunRepo) GetByToken(
	ctx context.Context,
	token string,
) (domain.AgentRun, error) {
	v, ok := r.tokenToID.Load(token)
	if !ok {
		return domain.AgentRun{}, fmt.Errorf("agentrun: get by token: %w", asynxModels.ErrNotFound)
	}
	id, _ := v.(string)
	return r.Get(ctx, id)
}

func (r *agentRunRepo) ListByTask(
	ctx context.Context,
	taskID string,
) ([]domain.AgentRun, error) {
	r.mu.Lock()
	ids := make([]string, len(r.ids))
	copy(ids, r.ids)
	r.mu.Unlock()

	var result []domain.AgentRun
	for _, id := range ids {
		run, err := r.ax.Get(ctx, id)
		if err != nil {
			continue
		}
		if run.TaskID == taskID {
			result = append(result, run)
		}
	}
	return result, nil
}

func (r *agentRunRepo) CompleteAgentRun(
	ctx context.Context,
	id string,
	output string,
) error {
	_, err := r.ax.Send(ctx, agentcmds.CompleteAgentRun{ID: id, Output: output})
	if err != nil {
		return fmt.Errorf("agentrun: complete %s: %w", id, err)
	}
	return nil
}

func (r *agentRunRepo) FailAgentRun(
	ctx context.Context,
	id string,
) error {
	_, err := r.ax.Send(ctx, agentcmds.FailAgentRun{ID: id})
	if err != nil {
		return fmt.Errorf("agentrun: fail %s: %w", id, err)
	}
	return nil
}

func (r *agentRunRepo) InterruptAgentRun(
	ctx context.Context,
	id string,
) error {
	_, err := r.ax.Send(ctx, agentcmds.InterruptAgentRun{ID: id})
	if err != nil {
		return fmt.Errorf("agentrun: interrupt %s: %w", id, err)
	}
	return nil
}

func (r *agentRunRepo) RecoverOrphanedRuns(
	ctx context.Context,
) error {
	// Merge IDs from the persistent store with any in-memory IDs.
	seen := make(map[string]bool)
	r.mu.Lock()
	for _, id := range r.ids {
		seen[id] = true
	}
	r.mu.Unlock()

	if r.idProvider != nil {
		persistedIDs, err := r.idProvider(ctx)
		if err != nil {
			slog.WarnContext(ctx, "agentrun: recover: list ids failed", "err", err)
		} else {
			r.mu.Lock()
			for _, id := range persistedIDs {
				if !seen[id] {
					r.ids = append(r.ids, id)
					seen[id] = true
				}
			}
			r.mu.Unlock()
		}
	}

	r.mu.Lock()
	ids := make([]string, len(r.ids))
	copy(ids, r.ids)
	r.mu.Unlock()

	for _, id := range ids {
		run, err := r.ax.Get(ctx, id)
		if err != nil {
			slog.WarnContext(ctx, "agentrun: recover: get failed", "id", id, "err", err)
			continue
		}

		if run.Token != "" {
			r.tokenToID.Store(run.Token, run.ID)
		}

		if run.Status != domain.AgentRunStatusRunning {
			continue
		}

		slog.InfoContext(ctx, "agentrun: recover: marking orphaned run as failed", "id", id)
		if sendErr := r.FailAgentRun(ctx, id); sendErr != nil {
			slog.WarnContext(ctx, "agentrun: recover: fail command failed", "id", id, "err", sendErr)
			continue
		}

		if r.taskPauser != nil {
			if pauseErr := r.taskPauser(ctx, run.TaskID); pauseErr != nil {
				slog.WarnContext(ctx, "agentrun: recover: pause task failed", "task_id", run.TaskID, "err", pauseErr)
			}
		}
	}
	return nil
}

func (r *agentRunRepo) Subscribe(
	topic string,
	fn func(ctx context.Context, evt asynxModels.Event[domain.AgentRun]),
) (string, error) {
	id, err := r.ax.Subscribe(asynx.Topic(topic), fn)
	if err != nil {
		return "", fmt.Errorf("agentrun: subscribe %q: %w", topic, err)
	}
	return id, nil
}
