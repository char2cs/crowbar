// Package agentchat is a bespoke gorm repository for the Crowbar-owned
// agentic chat/segment data model (domain.AgentChat / domain.AgentSegment).
// It is distinct from the dormant event-sourced domain.Chat.
package agentchat

import (
	"context"
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	gormdb "gorm.io/gorm"
)

// ErrNotFound is returned when no chat/segment row exists for the given ID.
var ErrNotFound = errors.New("agentchat: not found")

// Store persists AgentChat and AgentSegment rows.
//
// Invariant: at most one AgentSegment with status="active" exists per
// CrowbarSegmentID (one live process = one active segment; a chat move ends
// the old active segment and opens a new one). "The current segment for a
// process" must therefore always be resolved via GetActiveSegmentByCrowbarID,
// never a bare lookup on CrowbarSegmentID (multiple rows can share it once
// moves have happened).
type Store interface {
	SaveChat(ctx context.Context, c domain.AgentChat) error
	GetChat(ctx context.Context, id string) (domain.AgentChat, error)
	ListChats(ctx context.Context) ([]domain.AgentChat, error)
	SaveSegment(ctx context.Context, s domain.AgentSegment) error
	GetSegment(ctx context.Context, id string) (domain.AgentSegment, error)
	GetActiveSegmentByCrowbarID(ctx context.Context, crowbarSegID string) (domain.AgentSegment, error)
	ListSegmentsByChat(ctx context.Context, chatID string) ([]domain.AgentSegment, error)
	AllSegments(ctx context.Context) ([]domain.AgentSegment, error)
}

type gormStore struct{ db *gormdb.DB }

// New builds the agentchat store, auto-migrating the agent_chats and
// agent_segments tables.
func New(db *gormdb.DB) (Store, error) {
	if err := db.AutoMigrate(&domain.AgentChat{}, &domain.AgentSegment{}); err != nil {
		return nil, fmt.Errorf("agentchat: migrate: %w", err)
	}
	return &gormStore{db: db}, nil
}

func (s *gormStore) SaveChat(ctx context.Context, c domain.AgentChat) error {
	return s.db.WithContext(ctx).Save(&c).Error
}

func (s *gormStore) GetChat(ctx context.Context, id string) (domain.AgentChat, error) {
	var c domain.AgentChat
	if err := s.db.WithContext(ctx).First(&c, "id = ?", id).Error; err != nil {
		if errors.Is(err, gormdb.ErrRecordNotFound) {
			return domain.AgentChat{}, ErrNotFound
		}
		return domain.AgentChat{}, err
	}
	return c, nil
}

func (s *gormStore) ListChats(ctx context.Context) ([]domain.AgentChat, error) {
	var out []domain.AgentChat
	return out, s.db.WithContext(ctx).Find(&out).Error
}

func (s *gormStore) SaveSegment(ctx context.Context, seg domain.AgentSegment) error {
	return s.db.WithContext(ctx).Save(&seg).Error
}

func (s *gormStore) GetSegment(ctx context.Context, id string) (domain.AgentSegment, error) {
	var seg domain.AgentSegment
	if err := s.db.WithContext(ctx).First(&seg, "id = ?", id).Error; err != nil {
		if errors.Is(err, gormdb.ErrRecordNotFound) {
			return domain.AgentSegment{}, ErrNotFound
		}
		return domain.AgentSegment{}, err
	}
	return seg, nil
}

// GetActiveSegmentByCrowbarID returns the single active segment for a live process
// (its CrowbarSegmentID). Multiple rows may share a crowbar_segment_id after chat
// moves, but the invariant guarantees at most one is active — so callers resolving
// "the current segment for a process" MUST use this, never a bare .First on the id.
func (s *gormStore) GetActiveSegmentByCrowbarID(ctx context.Context, crowbarSegID string) (domain.AgentSegment, error) {
	var seg domain.AgentSegment
	err := s.db.WithContext(ctx).
		Where("crowbar_segment_id = ? AND status = ?", crowbarSegID, "active").
		Order("started_at desc").
		First(&seg).Error
	if err != nil {
		if errors.Is(err, gormdb.ErrRecordNotFound) {
			return domain.AgentSegment{}, ErrNotFound
		}
		return domain.AgentSegment{}, err
	}
	return seg, nil
}

func (s *gormStore) ListSegmentsByChat(ctx context.Context, chatID string) ([]domain.AgentSegment, error) {
	var out []domain.AgentSegment
	return out, s.db.WithContext(ctx).Where("chat_id = ?", chatID).Order("started_at asc").Find(&out).Error
}

func (s *gormStore) AllSegments(ctx context.Context) ([]domain.AgentSegment, error) {
	var out []domain.AgentSegment
	return out, s.db.WithContext(ctx).Find(&out).Error
}
