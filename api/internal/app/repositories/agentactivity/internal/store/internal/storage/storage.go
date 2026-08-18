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
		Seq: i.Seq, Kind: i.Kind, Detail: i.Detail, At: i.At, ResolvedAt: i.ResolvedAt,
	})
}

// SaveChoice writes one prompt, open or resolved.
//
// It upserts the WHOLE row on both phases, so a replay that re-delivers the open
// event and then the close event converges on the same values it did live. That
// is why the resolution sweeps below all filter on resolved_at IS NULL: a sweep
// that re-resolved an already-resolved row would move its timestamp on every
// rebuild, and the read model would stop being reproducible.
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
	})
}

// encodeList stores an empty list as an EMPTY STRING rather than as "null" or
// "[]", so "nothing was offered" is one value in the column instead of three the
// read side would each have to recognise.
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

// ResolveChoicesForTool closes every pending prompt that was gating a tool call
// which has just finished.
//
// It is the resolution that actually fires in production. A permission is
// answered at the PTY by a human typing into the vendor CLI, and no provider
// reports that: the only observable consequence is that the gated work proceeds.
// A prompt held open until an explicit answer arrived would therefore hang over a
// chat forever, which is strictly worse than clearing one moment early.
//
// The name is matched only for a prompt that never learned a call id, because a
// claude permission carries no tool_use_id at all (measured against 2.1.234 on
// 2026-08-17) and the correlating PreToolUse may have been lost.
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
		// An anonymous completion identifies no prompt. Sweeping on that would clear
		// every pending question in the chat, including ones about other tools.
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

// ResolveOpenChoices closes every prompt a chat still has pending.
//
// It runs at the turn boundary, and it is the backstop rather than the main path:
// an elicitation has no resolving event on any provider, and a permission whose
// tool never completed has nothing to be resolved by. A turn that has ended is not
// waiting on an answer, so nothing pending can survive it.
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

// RepointActivity moves every item attached to one turn id onto another.
//
// It runs once per turn boundary, over the handful of rows one turn produced, and
// it is what keeps a tool call attributable to the reply it produced rather than
// to the placeholder it was recorded against.
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
	for _, model := range []any{
		&TurnRow{}, &ToolCallRow{}, &SubagentRow{}, &InterruptionRow{}, &ChoiceRow{},
	} {
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
