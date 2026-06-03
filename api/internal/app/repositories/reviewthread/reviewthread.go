package reviewthread

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	"github.com/google/uuid"

	threadcmds "github.com/char2cs/crowbar/api/internal/app/repositories/reviewthread/internal/commands"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// ReviewThread is the repository interface for the ReviewThread aggregate.
type ReviewThread interface {
	Open(ctx context.Context, taskID string, agentRunID *string, file string, line int, phase domain.ReviewPhase, openedBy string, content string) (domain.ReviewThread, error)
	Get(ctx context.Context, id string) (domain.ReviewThread, error)
	ListByTask(ctx context.Context, taskID string, status string, phase string) ([]domain.ReviewThread, error)
	PostMessage(ctx context.Context, threadID string, role string, content string) error
	ForceApprove(ctx context.Context, threadID string) error
	ResolveThread(ctx context.Context, threadID string, emoji string) error
	Subscribe(topic string, fn func(ctx context.Context, evt asynxModels.Event[domain.ReviewThread])) (string, error)
}

type reviewThreadRepo struct {
	ax  asynx.Asynx[domain.ReviewThread]
	mu  sync.Mutex
	ids []string
}

// New constructs a reviewThreadRepo wrapping the provided Asynx instance.
func New(
	ax asynx.Asynx[domain.ReviewThread],
) ReviewThread {
	return &reviewThreadRepo{ax: ax}
}

func (r *reviewThreadRepo) Open(
	ctx context.Context,
	taskID string,
	agentRunID *string,
	file string,
	line int,
	phase domain.ReviewPhase,
	openedBy string,
	content string,
) (domain.ReviewThread, error) {
	id := uuid.NewString()
	now := time.Now()
	firstMsg := domain.ReviewMessage{
		ID:        uuid.NewString(),
		ThreadID:  id,
		Role:      openedBy,
		Content:   content,
		CreatedAt: now,
	}

	evt, err := r.ax.SendWait(ctx, threadcmds.OpenReviewThread{
		ID:         id,
		TaskID:     taskID,
		AgentRunID: agentRunID,
		File:       file,
		Line:       line,
		Phase:      phase,
		OpenedBy:   openedBy,
		FirstMsg:   firstMsg,
	})
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: open: %w", err)
	}

	r.mu.Lock()
	r.ids = append(r.ids, id)
	r.mu.Unlock()

	return evt.Aggregate, nil
}

func (r *reviewThreadRepo) Get(
	ctx context.Context,
	id string,
) (domain.ReviewThread, error) {
	t, err := r.ax.Get(ctx, id)
	if err != nil {
		return domain.ReviewThread{}, fmt.Errorf("reviewthread: get %s: %w", id, err)
	}
	return t, nil
}

func (r *reviewThreadRepo) ListByTask(
	ctx context.Context,
	taskID string,
	status string,
	phase string,
) ([]domain.ReviewThread, error) {
	r.mu.Lock()
	ids := make([]string, len(r.ids))
	copy(ids, r.ids)
	r.mu.Unlock()

	var result []domain.ReviewThread
	for _, id := range ids {
		t, err := r.ax.Get(ctx, id)
		if err != nil {
			continue
		}
		if t.TaskID != taskID {
			continue
		}
		if status != "" && string(t.Status) != status {
			continue
		}
		if phase != "" && string(t.Phase) != phase {
			continue
		}
		result = append(result, t)
	}
	return result, nil
}

func (r *reviewThreadRepo) PostMessage(
	ctx context.Context,
	threadID string,
	role string,
	content string,
) error {
	msg := domain.ReviewMessage{
		ID:        uuid.NewString(),
		ThreadID:  threadID,
		Role:      role,
		Content:   content,
		CreatedAt: time.Now(),
	}
	_, err := r.ax.Send(ctx, threadcmds.PostReviewThreadMessage{ThreadID: threadID, Message: msg})
	if err != nil {
		return fmt.Errorf("reviewthread: post message %s: %w", threadID, err)
	}
	return nil
}

func (r *reviewThreadRepo) ForceApprove(
	ctx context.Context,
	threadID string,
) error {
	_, err := r.ax.Send(ctx, threadcmds.ForceApproveReviewThread{ThreadID: threadID})
	if err != nil {
		return fmt.Errorf("reviewthread: force approve %s: %w", threadID, err)
	}
	return nil
}

func (r *reviewThreadRepo) ResolveThread(
	ctx context.Context,
	threadID string,
	emoji string,
) error {
	_, err := r.ax.Send(ctx, threadcmds.ResolveReviewThread{ThreadID: threadID, Emoji: emoji})
	if err != nil {
		return fmt.Errorf("reviewthread: resolve %s: %w", threadID, err)
	}
	return nil
}

func (r *reviewThreadRepo) Subscribe(
	topic string,
	fn func(ctx context.Context, evt asynxModels.Event[domain.ReviewThread]),
) (string, error) {
	subID, err := r.ax.Subscribe(asynx.Topic(topic), fn)
	if err != nil {
		return "", fmt.Errorf("reviewthread: subscribe %q: %w", topic, err)
	}
	return subID, nil
}
