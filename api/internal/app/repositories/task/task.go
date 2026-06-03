package task

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os/exec"
	"path/filepath"
	"sync"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	taskcmds "github.com/char2cs/crowbar/api/internal/app/repositories/task/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// Task is the repository interface for the Task aggregate.
type Task interface {
	Create(ctx context.Context, repositoryID string, title string, flowPath string, initialState string, branchName string, baseBranch string, repoPath string, worktreePath string) (domain.Task, error)
	Get(ctx context.Context, id string) (domain.Task, error)
	List(ctx context.Context, repositoryID string) ([]domain.Task, error)
	Archive(ctx context.Context, id string) error
	Pause(ctx context.Context, id string) error
	Resume(ctx context.Context, id string) error
	Retry(ctx context.Context, id string) error
	AdvanceState(ctx context.Context, id string, nextState string, event string) error
	ForceTransition(ctx context.Context, id string, toState string) error
	Subscribe(topic string, fn func(ctx context.Context, evt asynxModels.Event[domain.Task])) (string, error)
}

type taskRepo struct {
	ax  asynx.Asynx[domain.Task]
	mu  sync.Mutex
	ids map[string][]string
}

// New constructs a taskRepo wrapping the provided Asynx instance.
func New(
	ax asynx.Asynx[domain.Task],
) Task {
	return &taskRepo{ax: ax, ids: make(map[string][]string)}
}

func (r *taskRepo) Create(
	ctx context.Context,
	repositoryID string,
	title string,
	flowPath string,
	initialState string,
	branchName string,
	baseBranch string,
	repoPath string,
	worktreePath string,
) (domain.Task, error) {
	cmd := exec.CommandContext(ctx, "git", "worktree", "add", worktreePath, "-b", branchName)
	cmd.Dir = repoPath
	if out, err := cmd.CombinedOutput(); err != nil {
		return domain.Task{}, fmt.Errorf("task: git worktree add: %w: %s", err, out)
	}

	id := uuid.NewString()
	evt, err := r.ax.SendWait(ctx, taskcmds.CreateTask{
		ID:           id,
		RepositoryID: repositoryID,
		Title:        title,
		FlowPath:     flowPath,
		InitialState: initialState,
		BranchName:   branchName,
		BaseBranch:   baseBranch,
		WorktreePath: worktreePath,
	})
	if err != nil {
		return domain.Task{}, fmt.Errorf("task: create: %w", err)
	}

	r.mu.Lock()
	r.ids[repositoryID] = append(r.ids[repositoryID], id)
	r.mu.Unlock()

	return evt.Aggregate, nil
}

func (r *taskRepo) List(
	ctx context.Context,
	repositoryID string,
) ([]domain.Task, error) {
	r.mu.Lock()
	ids := append([]string(nil), r.ids[repositoryID]...)
	r.mu.Unlock()

	tasks := make([]domain.Task, 0, len(ids))
	for _, id := range ids {
		t, err := r.ax.Get(ctx, id)
		if err != nil {
			return nil, fmt.Errorf("task: list get %s: %w", id, err)
		}
		tasks = append(tasks, t)
	}
	return tasks, nil
}

func (r *taskRepo) Get(
	ctx context.Context,
	id string,
) (domain.Task, error) {
	t, err := r.ax.Get(ctx, id)
	if err != nil {
		if errors.Is(err, asynxModels.ErrNotFound) {
			return domain.Task{}, fmt.Errorf("task: get %s: %w", id, domain.ErrNotFound)
		}
		return domain.Task{}, fmt.Errorf("task: get %s: %w", id, err)
	}
	return t, nil
}

func (r *taskRepo) Archive(
	ctx context.Context,
	id string,
) error {
	t, err := r.ax.Get(ctx, id)
	if err != nil {
		return fmt.Errorf("task: archive get %s: %w", id, err)
	}

	if t.WorktreePath != "" {
		// WorktreePath is <repoRoot>/.worktrees/<branch>; derive the repo root.
		repoRoot := filepath.Dir(filepath.Dir(t.WorktreePath))
		cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", t.WorktreePath)
		cmd.Dir = repoRoot
		if out, err := cmd.CombinedOutput(); err != nil {
			slog.WarnContext(ctx, "task: archive: git worktree remove", "id", id, "err", err, "output", string(out))
		}
	}

	_, err = r.ax.Send(ctx, taskcmds.ArchiveTask{ID: id})
	if err != nil {
		return fmt.Errorf("task: archive %s: %w", id, err)
	}
	return nil
}

func (r *taskRepo) Pause(
	ctx context.Context,
	id string,
) error {
	_, err := r.ax.Send(ctx, taskcmds.PauseTask{ID: id})
	if err != nil {
		return fmt.Errorf("task: pause %s: %w", id, err)
	}
	return nil
}

func (r *taskRepo) Resume(
	ctx context.Context,
	id string,
) error {
	_, err := r.ax.Send(ctx, taskcmds.ResumeTask{ID: id})
	if err != nil {
		return fmt.Errorf("task: resume %s: %w", id, err)
	}
	return nil
}

func (r *taskRepo) Retry(
	ctx context.Context,
	id string,
) error {
	_, err := r.ax.Send(ctx, taskcmds.RetryTask{ID: id})
	if err != nil {
		return fmt.Errorf("task: retry %s: %w", id, err)
	}
	return nil
}

func (r *taskRepo) AdvanceState(
	ctx context.Context,
	id string,
	nextState string,
	event string,
) error {
	_, err := r.ax.Send(ctx, taskcmds.AdvanceState{ID: id, NextState: nextState, Event: event})
	if err != nil {
		return fmt.Errorf("task: advance state %s: %w", id, err)
	}
	return nil
}

func (r *taskRepo) ForceTransition(
	ctx context.Context,
	id string,
	toState string,
) error {
	_, err := r.ax.Send(ctx, taskcmds.ForceTransition{ID: id, ToState: toState})
	if err != nil {
		return fmt.Errorf("task: force transition %s: %w", id, err)
	}
	return nil
}

func (r *taskRepo) Subscribe(
	topic string,
	fn func(ctx context.Context, evt asynxModels.Event[domain.Task]),
) (string, error) {
	id, err := r.ax.Subscribe(asynx.Topic(topic), fn)
	if err != nil {
		return "", fmt.Errorf("task: subscribe %q: %w", topic, err)
	}
	return id, nil
}
