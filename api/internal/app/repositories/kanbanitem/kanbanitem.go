package kanbanitem

import (
	"context"
	"fmt"
	"sync"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	kanbancmds "github.com/char2cs/crowbar/api/internal/app/repositories/kanbanitem/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// KanbanItem is the repository interface for the KanbanItem aggregate.
type KanbanItem interface {
	Create(ctx context.Context, taskID string, title string, agentRunID string) (domain.KanbanItem, error)
	Get(ctx context.Context, id string) (domain.KanbanItem, error)
	ListByTask(ctx context.Context, taskID string) ([]domain.KanbanItem, error)
	UpdateStatus(ctx context.Context, id string, status string, agentRunID string) error
	Subscribe(topic string, fn func(ctx context.Context, evt asynxModels.Event[domain.KanbanItem])) (string, error)
}

type kanbanItemRepo struct {
	ax  asynx.Asynx[domain.KanbanItem]
	mu  sync.Mutex
	ids []string
}

// New constructs a kanbanItemRepo wrapping the provided Asynx instance.
func New(
	ax asynx.Asynx[domain.KanbanItem],
) KanbanItem {
	return &kanbanItemRepo{ax: ax}
}

func (r *kanbanItemRepo) Create(
	ctx context.Context,
	taskID string,
	title string,
	agentRunID string,
) (domain.KanbanItem, error) {
	id := uuid.NewString()
	evt, err := r.ax.SendWait(ctx, kanbancmds.CreateKanbanItem{
		ID:         id,
		TaskID:     taskID,
		Title:      title,
		AgentRunID: agentRunID,
	})
	if err != nil {
		return domain.KanbanItem{}, fmt.Errorf("kanbanitem: create: %w", err)
	}

	r.mu.Lock()
	r.ids = append(r.ids, id)
	r.mu.Unlock()

	return evt.Aggregate, nil
}

func (r *kanbanItemRepo) Get(
	ctx context.Context,
	id string,
) (domain.KanbanItem, error) {
	item, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.KanbanItem{}, fmt.Errorf("kanbanitem: get %s: %w", id, err)
	}
	return item, nil
}

func (r *kanbanItemRepo) ListByTask(
	ctx context.Context,
	taskID string,
) ([]domain.KanbanItem, error) {
	r.mu.Lock()
	ids := make([]string, len(r.ids))
	copy(ids, r.ids)
	r.mu.Unlock()

	var result []domain.KanbanItem
	for _, id := range ids {
		item, err := r.ax.Get(ctx, id)
		if err != nil {
			continue
		}
		if item.TaskID == taskID {
			result = append(result, item)
		}
	}
	return result, nil
}

func (r *kanbanItemRepo) UpdateStatus(
	ctx context.Context,
	id string,
	status string,
	agentRunID string,
) error {
	_, err := r.ax.Send(ctx, kanbancmds.UpdateKanbanItemStatus{ID: id, Status: status, AgentRunID: agentRunID})
	if err != nil {
		return fmt.Errorf("kanbanitem: update status %s: %w", id, err)
	}
	return nil
}

func (r *kanbanItemRepo) Subscribe(
	topic string,
	fn func(ctx context.Context, evt asynxModels.Event[domain.KanbanItem]),
) (string, error) {
	subID, err := r.ax.Subscribe(asynx.Topic(topic), fn)
	if err != nil {
		return "", fmt.Errorf("kanbanitem: subscribe %q: %w", topic, err)
	}
	return subID, nil
}
