package conversation

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	gormdb "gorm.io/gorm"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Conversation is the repository interface for ConversationMessage records.
type Conversation interface {
	Create(ctx context.Context, taskID string, role domain.ConversationMessageRole, msgType domain.ConversationMessageType, content string) (domain.ConversationMessage, error)
	ListByTask(ctx context.Context, taskID string) ([]domain.ConversationMessage, error)
}

type conversationRepo struct {
	db *gormdb.DB
}

// New constructs a GORM-backed Conversation repository and runs AutoMigrate.
func New(
	db *gormdb.DB,
) (Conversation, error) {
	if err := db.AutoMigrate(&domain.ConversationMessage{}); err != nil {
		return nil, fmt.Errorf("conversation repo: migrate: %w", err)
	}
	return &conversationRepo{db: db}, nil
}

func (r *conversationRepo) Create(
	ctx context.Context,
	taskID string,
	role domain.ConversationMessageRole,
	msgType domain.ConversationMessageType,
	content string,
) (domain.ConversationMessage, error) {
	msg := domain.ConversationMessage{
		ID:        uuid.New().String(),
		TaskID:    taskID,
		Role:      role,
		Type:      msgType,
		Content:   content,
		CreatedAt: time.Now().UTC(),
	}
	if err := r.db.WithContext(ctx).Create(&msg).Error; err != nil {
		return domain.ConversationMessage{}, fmt.Errorf("conversation repo: create: %w", err)
	}
	return msg, nil
}

func (r *conversationRepo) ListByTask(
	ctx context.Context,
	taskID string,
) ([]domain.ConversationMessage, error) {
	var msgs []domain.ConversationMessage
	if err := r.db.WithContext(ctx).
		Where("task_id = ?", taskID).
		Order("created_at ASC").
		Find(&msgs).Error; err != nil {
		return nil, fmt.Errorf("conversation repo: list by task: %w", err)
	}
	return msgs, nil
}
