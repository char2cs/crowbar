// This file holds the THIRD projection of the runner event stream: append-only
// PLACEMENT history — which providers have been pointed at a chat, and when each
// one last arrived there.
//
// It is the placement twin of the conversation history in store.go, and it
// exists because that one is blind by construction. appendConversation can only
// record a (chat, conversation) pair, so it records NOTHING for a provider that
// binds via its own connection identity and never announces a conversation. A
// chat that only ever ran such a provider — and was never switched, so it has no
// interruption marker either — left no trace in any read model at all, and every
// resolver answered "no provider has ever run here" for a chat whose CLI had
// been running in it for days.
//
// A placement is recorded at the moment Crowbar points a runner at a chat, which
// happens before the provider has announced anything and happens for EVERY
// runner. That is the whole design: it derives from the same events, it is
// append-only for the same reason (a dormant chat must still be able to say what
// it ran), and it needs no cooperation from the vendor CLI.
package store

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// placementRow is APPEND-ONLY placement history: one row per (chat, provider)
// pair any runner has ever been placed on, never deleted except by the chat
// delete cascade (ForgetChat), exactly like conversationRow.
//
// The key is (chat, provider) rather than (chat, runner) because the question it
// answers is "which VENDOR was last running here", not "which process": every
// respawn of the same provider on the same chat is the same answer, and folding
// them into one row keeps the table bounded by the chats a user actually has
// rather than by every CLI they have ever started.
type placementRow struct {
	ChatID      string `gorm:"primaryKey;index"`
	ProviderID  string `gorm:"primaryKey"`
	WorkspaceID string `gorm:"index"`
	// LastPlacedAt advances on every re-placement of this provider on this chat
	// and never moves backwards — see appendPlacement's MAX.
	LastPlacedAt time.Time
}

func (placementRow) TableName() string {
	return "agent_chat_placements"
}

func (p placementRow) toPlacement() agents.ChatPlacement {
	return agents.ChatPlacement{
		ChatID:       p.ChatID,
		ProviderID:   p.ProviderID,
		LastPlacedAt: p.LastPlacedAt,
	}
}

// PlacementsForChat returns every provider that has ever been placed on chatID,
// OLDEST ARRIVAL FIRST — from append-only history, so it keeps answering long
// after the runners that were placed there have exited.
//
// It is the read that makes a dormant chat's provider recoverable when nothing
// else knows it: no conversation (the provider never announced one), no switch
// marker (it was never switched) and no durable choice on the chat itself (it
// was minted before that field existed). An empty result means no CLI has ever
// been started on this chat at all, which is a real and distinguishable answer,
// not a lookup failure — so this returns no ErrNotFound.
//
// An empty chatID is NOWHERE, and nowhere has held nobody.
func (s *Store) PlacementsForChat(
	ctx context.Context,
	chatID string,
) ([]agents.ChatPlacement, error) {
	if chatID == "" {
		return nil, nil
	}
	var rows []placementRow
	err := s.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Order("last_placed_at ASC, provider_id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentrunner store: placements for chat %q: %w", chatID, err)
	}
	out := make([]agents.ChatPlacement, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toPlacement())
	}
	return out, nil
}

// forgetPlacements drops a chat's placement history — the ForgetChat half of the
// delete cascade for this projection, and, as for the conversation history, the
// only thing permitted to remove append-only rows.
func forgetPlacements(
	ctx context.Context,
	db *gormdb.DB,
	chatID string,
) error {
	if err := db.WithContext(ctx).Delete(&placementRow{}, "chat_id = ?", chatID).Error; err != nil {
		return fmt.Errorf("agentrunner store: forget placements for chat %q: %w", chatID, err)
	}
	return nil
}

// appendPlacement records that a runner of r's provider is pointed at r's chat,
// stamped with when it ARRIVED there (arrivedAt). Idempotent on (chat,
// provider): a respawn, a rebind and a move back all restate the same pair and
// only move the timestamp forward.
//
// A runner placed NOWHERE writes nothing. That is not a special case, it is the
// point: a displaced runner (the outgoing half of a switch, a chat deleted under
// it) points at no chat, and recording it against "" would invent a placement on
// a chat that does not exist — the same lie LiveRunnerForChat refuses to tell
// for the same reason.
//
// The timestamp is written with MAX rather than assigned, so an event projected
// out of order cannot drag a provider's arrival backwards. Ordering only matters
// ACROSS providers — the resolver picks the newest arrival among them — and two
// runners of different providers landing on one chat concurrently is exactly the
// eviction window the live model already exists to survive.
//
// Package-level rather than a projector method for the same reason
// appendConversation is: the boot heal folds it WITHOUT the live-row writer.
func appendPlacement(
	ctx context.Context,
	db *gormdb.DB,
	r agents.Runner,
) error {
	if r.CurrentChatID == "" || r.ProviderID == "" {
		return nil
	}
	row := placementRow{
		ChatID:       r.CurrentChatID,
		ProviderID:   r.ProviderID,
		WorkspaceID:  r.WorkspaceID,
		LastPlacedAt: arrivedAt(r),
	}
	err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "chat_id"}, {Name: "provider_id"}},
		DoUpdates: clause.Assignments(map[string]any{
			"last_placed_at": gormdb.Expr(
				"MAX(agent_chat_placements.last_placed_at, excluded.last_placed_at)"),
		}),
	}).Create(&row).Error
	if err != nil {
		return fmt.Errorf("agentrunner store: append placement (chat %q, provider %q): %w",
			r.CurrentChatID, r.ProviderID, err)
	}
	return nil
}

// arrivedAt is when this runner reached where it currently is: the later of its
// spawn and its move into its current conversation. It is the Go twin of the
// newestArrivalFirst ordering the live model uses, and it is deliberately the
// same instant a conversation row is stamped with, so placement timestamps and
// conversation timestamps order against each other correctly in one scan.
func arrivedAt(
	r agents.Runner,
) time.Time {
	if r.CurrentSessionSince.After(r.StartedAt) {
		return r.CurrentSessionSince
	}
	return r.StartedAt
}
