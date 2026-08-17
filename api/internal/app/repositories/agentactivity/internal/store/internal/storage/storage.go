package storage

import (
	"context"
	"fmt"
	"time"

	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// Store is the read model's write and query surface.
//
// Every write is an UPSERT keyed by the row's own identity, which is what makes
// the projection idempotent: replaying an event that was already projected
// rewrites the same row with the same values, so a rebuild needs no watermark and
// cannot double-count.
type Store struct {
	db *gormdb.DB
}

func New(db *gormdb.DB) (*Store, error) {
	if err := db.AutoMigrate(
		&TurnRow{}, &ToolCallRow{}, &SubagentRow{}, &InterruptionRow{},
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
		Status: c.Status, DurationMS: c.DurationMS,
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
		Seq: i.Seq, Kind: i.Kind, Detail: i.Detail, At: i.At, ResolvedAt: i.ResolvedAt,
	})
}

// RepointActivity moves every item attached to one turn id onto another.
//
// It runs once per turn boundary, over the handful of rows one turn produced, and
// it is what keeps a tool call attributable to the reply it produced rather than
// to the placeholder it was recorded against.
func (s *Store) RepointActivity(ctx context.Context, chatID, from, to string) error {
	db := s.db.WithContext(ctx)
	for _, model := range []any{&ToolCallRow{}, &SubagentRow{}, &InterruptionRow{}} {
		if err := db.Model(model).
			Where("chat_id = ? AND turn_id = ?", chatID, from).
			Update("turn_id", to).Error; err != nil {
			return fmt.Errorf("agentactivity storage: repoint activity: %w", err)
		}
	}
	return nil
}

// AbandonRunningTools marks every still-running tool call of a chat as abandoned.
//
// It runs when a turn ends, because the providers do not guarantee a completion
// for every invocation: a tool whose post hook never arrives would otherwise show
// as running forever, and "running for 3 days" is a worse lie than "abandoned".
func (s *Store) AbandonRunningTools(ctx context.Context, chatID string, endedAt *time.Time) error {
	return s.db.WithContext(ctx).Model(&ToolCallRow{}).
		Where("chat_id = ? AND status = ?", chatID, domain.ToolStatusRunning).
		Updates(map[string]any{
			"status":   domain.ToolStatusAbandoned,
			"ended_at": endedAt,
		}).Error
}

// ResolveOpenInterruptions closes every interruption a chat still has open.
//
// It runs at the turn boundary because an interruption cannot outlive its turn —
// a permission wait, a notification and a compaction all end when the turn does.
// It is not optional politeness: claude's Notification has NO resolving event of
// its own, so without this sweep a single notification renders "the agent needs
// your attention" for the rest of the chat's life.
func (s *Store) ResolveOpenInterruptions(ctx context.Context, chatID string, at *time.Time) error {
	return s.db.WithContext(ctx).Model(&InterruptionRow{}).
		Where("chat_id = ? AND resolved_at IS NULL", chatID).
		Update("resolved_at", at).Error
}

// DeleteChat removes every row belonging to a chat. It is the purge path: a
// forgotten aggregate must not leave its conversation readable.
func (s *Store) DeleteChat(ctx context.Context, chatID string) error {
	db := s.db.WithContext(ctx)
	for _, model := range []any{&TurnRow{}, &ToolCallRow{}, &SubagentRow{}, &InterruptionRow{}} {
		if err := db.Where("chat_id = ?", chatID).Delete(model).Error; err != nil {
			return fmt.Errorf("agentactivity storage: delete chat: %w", err)
		}
	}
	return nil
}

// Empty reports whether the read model holds nothing at all, which is what a lazy
// heal checks before deciding to replay the log.
func (s *Store) Empty(ctx context.Context) (bool, error) {
	var count int64
	if err := s.db.WithContext(ctx).Model(&TurnRow{}).Limit(1).Count(&count).Error; err != nil {
		return false, fmt.Errorf("agentactivity storage: count: %w", err)
	}
	return count == 0, nil
}
