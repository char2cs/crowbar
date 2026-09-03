package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/char2cs/crowbar/api/internal/domain"
)

type Store struct {
	db *gormdb.DB
}

func New(db *gormdb.DB) (*Store, error) {
	if err := db.AutoMigrate(
		&TurnRow{}, &ToolCallRow{}, &SubagentRow{}, &InterruptionRow{}, &ChoiceRow{},
	); err != nil {
		return nil, fmt.Errorf("agentactivity storage: migrate: %w", err)
	}
	return &Store{db: db}, nil
}

func upsert[T any](ctx context.Context, db *gormdb.DB, row T) error {
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		UpdateAll: true,
	}).Create(&row).Error
}

func (s *Store) SaveTurn(ctx context.Context, t domain.ActivityTurn) error {
	return upsert(ctx, s.db, TurnRow{
		Key: rowKey(t.ChatID, t.ID), ID: t.ID, ChatID: t.ChatID, Seq: t.Seq,
		DisplayOrder: t.DisplayOrder, ItemIndex: t.ItemIndex,
		Role: t.Role, ProviderID: t.ProviderID, RunnerID: t.RunnerID,
		SessionID: t.SessionID, Text: t.Text, Effort: t.Effort,
		StartedAt: t.StartedAt, EndedAt: t.EndedAt,
	})
}

func (s *Store) SaveToolCall(ctx context.Context, c domain.ActivityToolCall) error {
	return upsert(ctx, s.db, ToolCallRow{
		Key: rowKey(c.ChatID, c.ID), ID: c.ID, TurnID: c.TurnID, ChatID: c.ChatID,
		Seq: c.Seq, Name: c.Name, Target: c.Target,
		RequestRef: c.RequestRef, ResultRef: c.ResultRef,
		Status: c.Status, Error: c.Error, DurationMS: c.DurationMS,
		StartedAt: c.StartedAt, EndedAt: c.EndedAt,
	})
}

func (s *Store) SaveSubagent(ctx context.Context, a domain.ActivitySubagent) error {
	return upsert(ctx, s.db, SubagentRow{
		Key: rowKey(a.ChatID, a.ID), ID: a.ID, TurnID: a.TurnID, ChatID: a.ChatID,
		Seq: a.Seq, AgentType: a.AgentType, StartedAt: a.StartedAt, EndedAt: a.EndedAt,
	})
}

func (s *Store) SaveInterruption(ctx context.Context, i domain.ActivityInterruption) error {
	return upsert(ctx, s.db, InterruptionRow{
		Key: rowKey(i.ChatID, i.ID), ID: i.ID, TurnID: i.TurnID, ChatID: i.ChatID,
		Seq: i.Seq, DisplayOrder: i.DisplayOrder,
		Kind: i.Kind, Detail: i.Detail, At: i.At, ResolvedAt: i.ResolvedAt,
	})
}

func (s *Store) SaveChoice(ctx context.Context, c domain.ActivityChoice) error {
	options, err := encodeList(c.Options)
	if err != nil {
		return fmt.Errorf("agentactivity storage: encode choice options: %w", err)
	}
	questions, err := encodeList(c.Questions)
	if err != nil {
		return fmt.Errorf("agentactivity storage: encode choice questions: %w", err)
	}
	return upsert(ctx, s.db, ChoiceRow{
		Key: rowKey(c.ChatID, c.ID), ID: c.ID, TurnID: c.TurnID, ChatID: c.ChatID,
		Seq: c.Seq, Kind: c.Kind, PromptID: c.PromptID,
		ToolID: c.ToolID, ToolName: c.ToolName,
		Title: c.Title, Question: c.Question, Mode: c.Mode, Multi: c.Multi,
		Options: options, Questions: questions, Schema: c.Schema,
		At: c.At, ResolvedAt: c.ResolvedAt, Resolution: c.Resolution,
		AutoApproved: c.AutoApproved,
	})
}

func encodeList[T any](items []T) (string, error) {
	if len(items) == 0 {
		return "", nil
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (s *Store) ResolveChoicesForTool(
	ctx context.Context,
	chatID, toolID, toolName string,
	at *time.Time,
) error {
	q := s.db.WithContext(ctx).Model(&ChoiceRow{}).
		Where("chat_id = ? AND resolved_at IS NULL", chatID)
	switch {
	case toolID != "" && toolName != "":
		q = q.Where("tool_id = ? OR (tool_id = '' AND tool_name = ?)", toolID, toolName)
	case toolID != "":
		q = q.Where("tool_id = ?", toolID)
	case toolName != "":
		q = q.Where("tool_id = '' AND tool_name = ?", toolName)
	default:

		return nil
	}
	err := q.Updates(map[string]any{
		"resolved_at": at,
		"resolution":  domain.ChoiceResolutionProceeded,
	}).Error
	if err != nil {
		return fmt.Errorf("agentactivity storage: resolve choices for tool: %w", err)
	}
	return nil
}

func (s *Store) ResolveOpenChoices(ctx context.Context, chatID string, at *time.Time) error {
	err := s.db.WithContext(ctx).Model(&ChoiceRow{}).
		Where("chat_id = ? AND resolved_at IS NULL", chatID).
		Updates(map[string]any{
			"resolved_at": at,
			"resolution":  domain.ChoiceResolutionAbandoned,
		}).Error
	if err != nil {
		return fmt.Errorf("agentactivity storage: resolve open choices: %w", err)
	}
	return nil
}

func (s *Store) RepointActivity(ctx context.Context, chatID, from, to string) error {
	db := s.db.WithContext(ctx)
	for _, model := range []any{&ToolCallRow{}, &SubagentRow{}, &InterruptionRow{}, &ChoiceRow{}} {
		if err := db.Model(model).
			Where("chat_id = ? AND turn_id = ?", chatID, from).
			Update("turn_id", to).Error; err != nil {
			return fmt.Errorf("agentactivity storage: repoint activity: %w", err)
		}
	}
	return nil
}

func (s *Store) AbandonRunningTools(ctx context.Context, chatID string, endedAt *time.Time) error {
	return s.db.WithContext(ctx).Model(&ToolCallRow{}).
		Where("chat_id = ? AND status = ?", chatID, domain.ToolStatusRunning).
		Updates(map[string]any{
			"status":   domain.ToolStatusAbandoned,
			"ended_at": endedAt,
		}).Error
}

func (s *Store) ResolveOpenInterruptions(ctx context.Context, chatID string, at *time.Time) error {
	return s.db.WithContext(ctx).Model(&InterruptionRow{}).
		Where("chat_id = ? AND resolved_at IS NULL", chatID).
		Update("resolved_at", at).Error
}

func (s *Store) DeleteChat(ctx context.Context, chatID string) error {
	db := s.db.WithContext(ctx)
	for _, model := range []any{
		&TurnRow{}, &ToolCallRow{}, &SubagentRow{}, &InterruptionRow{}, &ChoiceRow{},
	} {
		if err := db.Where("chat_id = ?", chatID).Delete(model).Error; err != nil {
			return fmt.Errorf("agentactivity storage: delete chat: %w", err)
		}
	}
	return nil
}

func (s *Store) Empty(ctx context.Context) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&TurnRow{}).Limit(1).Count(&count).Error; err != nil {
		return false, fmt.Errorf("agentactivity storage: count: %w", err)
	}
	return count == 0, nil
}
