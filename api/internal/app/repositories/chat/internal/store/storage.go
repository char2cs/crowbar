package store

import (
	"context"
	"encoding/json"
	"fmt"

	gormdb "gorm.io/gorm"

	storesqlite "github.com/char2cs/crowbar/api/internal/adapter/store/sqlite"
	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// agentChatRow is the durable read-model row for one AgentChat in
// agent_chats_read. WorkspaceID is indexed for a WHERE workspace_id = ? query,
// though ListByWorkspace filters in Go while chats-per-workspace stay few.
type agentChatRow struct {
	ID          string `gorm:"primaryKey;column:id"`
	WorkspaceID string `gorm:"column:workspace_id;index"`
	Data        []byte `gorm:"column:data"`
}

func (agentChatRow) TableName() string {
	return "agent_chats_read"
}

type storage interface {
	Save(
		ctx context.Context,
		chat domain.Chat,
	) error
	Delete(
		ctx context.Context,
		id string,
	) error
	FindByKey(
		ctx context.Context,
		id string,
	) (*domain.Chat, error)
	// FindAll answers every row that decodes, and the ids of those that do not.
	FindAll(
		ctx context.Context,
	) ([]domain.Chat, []string, error)
}

type storageStore struct {
	inner interface {
		Save(ctx context.Context, row agentChatRow) error
		Delete(ctx context.Context, key string) error
		FindByKey(ctx context.Context, key string) (*agentChatRow, error)
		FindAll(ctx context.Context) ([]agentChatRow, error)
	}
	unreadable serialize.Unreadable
}

func newStorageStore(
	db *gormdb.DB,
) (storage, error) {
	inner, err := storesqlite.NewFromDB[agentChatRow, string](db)
	if err != nil {
		return nil, fmt.Errorf("agentchat storage: %w", err)
	}
	return &storageStore{inner: inner}, nil
}

func (s *storageStore) Save(
	ctx context.Context,
	chat domain.Chat,
) error {
	data, err := json.Marshal(chat)
	if err != nil {
		return fmt.Errorf("agentchat storage: marshal: %w", err)
	}
	return s.inner.Save(ctx, agentChatRow{ID: chat.ID, WorkspaceID: chat.WorkspaceID, Data: data})
}

func (s *storageStore) Delete(
	ctx context.Context,
	id string,
) error {
	return s.inner.Delete(ctx, id)
}

func (s *storageStore) FindByKey(
	ctx context.Context,
	id string,
) (*domain.Chat, error) {
	row, err := s.inner.FindByKey(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("agentchat storage: find: %w", err)
	}
	if row == nil {
		return nil, nil
	}
	return unmarshalAgentChat(row.Data)
}

func (s *storageStore) FindAll(
	ctx context.Context,
) ([]domain.Chat, []string, error) {
	rows, err := s.inner.FindAll(ctx)
	if err != nil {
		return nil, nil, fmt.Errorf("agentchat storage: find all: %w", err)
	}
	result := make([]domain.Chat, 0, len(rows))
	var undecodable []string
	for _, row := range rows {
		chat, err := unmarshalAgentChat(row.Data)
		if err != nil {
			s.unreadable.Skip(ctx, "chat", row.ID, err)
			undecodable = append(undecodable, row.ID)
			continue
		}
		result = append(result, *chat)
	}
	return result, undecodable, nil
}

func unmarshalAgentChat(
	data []byte,
) (*domain.Chat, error) {
	var chat domain.Chat
	if err := json.Unmarshal(data, &chat); err != nil {
		return nil, fmt.Errorf("agentchat storage: unmarshal: %w", err)
	}
	return &chat, nil
}
