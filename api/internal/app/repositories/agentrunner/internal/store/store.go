// Package store owns the agentrunner read side: TWO projections folded from the
// AgentRunner event stream, which together replace everything AgentSegment used
// to be asked.
//
//  1. agent_runners (runnerRow) — LIVE rows. One row per RUNNING CLI, deleted on
//     exit. "Does a row exist for this chat" IS the liveness question: there is
//     no status column, so there is nothing to go stale. This is the core rule of
//     the refactor — we persist PLACEMENT (which chat, which conversation), never
//     LIVENESS. Liveness belongs to the PTY alone. Two authorities on liveness
//     always drift, and that drift is the bug being deleted here (today a segment
//     can read "ended" while its CLI is demonstrably still running).
//
//  2. agent_chat_conversations (conversationRow) — APPEND-ONLY history. Every
//     conversation a chat has ever hosted. Never updated, never deleted except by
//     the chat delete cascade (ForgetChat). It survives the process, so a dormant
//     chat stays resumable and its old conversations stay recognisable on a later
//     /resume. Append-only history cannot drift from reality; only live state can
//     — which is precisely why this one is safe to persist while liveness is not.
//
// Both derive independently from the same event stream, alongside the hub
// projection (hub.go) that owns WS fan-out — so none of them can drift from each
// other either.
package store

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/char2cs/crowbar/api/internal/app/repositories/internal/serialize"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// eventKeyPrefix is the namespace asynx prepends to an aggregate id when it
// stores that aggregate's events (its snapshots use "snapshots:"). The crowbar
// event store's AggregateLister returns the RAW store keys, so the rebuild must
// keep only the events keys and strip this prefix to recover the real aggregate
// id, which asynx.Replay re-prepends itself.
const eventKeyPrefix = "events:"

// ErrNotFound is returned when the read model holds no row for a query: no live
// runner for a chat/session/id, or no conversation for a session/chat. Defined
// locally so the parent agentrunner package (which imports this one) can bridge
// it outward to its own sentinel without an import cycle — the same layering
// agentchat keeps between its store and its event_store.go (mapNotFound).
//
// For the LIVE-runner reads this sentinel is not an error condition at all: the
// absence of a row IS the answer, and the answer is "dormant".
var ErrNotFound = errors.New("agentrunner store: not found")

// Store is the AgentRunner read side: the live-runner model and the append-only
// conversation history, both projected from the runner event stream.
type Store struct {
	db *gormdb.DB
	es asynxModels.Store
	ax asynx.Asynx[domain.AgentRunner]
}

// New builds both read models over db, then registers the store projection (this
// file) and the hub broadcast projection (hub.go) on ax. It does no eager
// reconcile: a normal boot re-opens the durable read model with zero replay.
// es is the same per-type event log ax wraps, retained so AllLive can heal a lost
// read model via whole-model Replay before boot reconciliation concludes that
// nothing is running.
func New(
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[domain.AgentRunner],
	broadcast BroadcastFunc,
) (*Store, error) {
	if err := db.AutoMigrate(&runnerRow{}, &conversationRow{}); err != nil {
		return nil, fmt.Errorf("agentrunner store: migrate: %w", err)
	}
	s := &Store{db: db, es: es, ax: ax}
	if err := registerStoreProjection(db, ax); err != nil {
		return nil, fmt.Errorf("agentrunner store: projections: %w", err)
	}
	if err := registerHubProjection(ax, broadcast); err != nil {
		return nil, fmt.Errorf("agentrunner store: projections: %w", err)
	}
	return s, nil
}

// LiveRunnerForChat returns the runner currently pointed at chatID, or
// ErrNotFound when the chat is dormant. There is no status column to consult:
// the row exists exactly while its CLI runs, so its presence IS liveness.
//
// It deliberately does NOT rebuild the read model on a miss (unlike AllLive):
// "no row" is the expected, overwhelmingly common answer for a dormant chat, and
// replaying the whole event log on every dormant read would put the entire
// history on the hot path.
func (s *Store) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (domain.AgentRunner, error) {
	var row runnerRow
	err := s.db.WithContext(ctx).Where("current_chat_id = ?", chatID).Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner store: live runner for chat %q: %w", chatID, ErrNotFound)
	}
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner store: live runner for chat %q: %w", chatID, err)
	}
	return row.toRunner(), nil
}

// LiveRunnerForSession returns the runner currently holding conversation
// sessionID in wsID — the incumbent the eviction path must displace (invariant
// I3: at most one live runner per conversation). ErrNotFound means nobody is
// running that conversation right now, which is not the same as "it never
// existed" — that question is ChatForSession's, and it is answered from history.
func (s *Store) LiveRunnerForSession(
	ctx context.Context,
	wsID string,
	sessionID string,
) (domain.AgentRunner, error) {
	var row runnerRow
	err := s.db.WithContext(ctx).
		Where("workspace_id = ? AND current_session = ?", wsID, sessionID).
		Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner store: live runner for session %q: %w", sessionID, ErrNotFound)
	}
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner store: live runner for session %q: %w", sessionID, err)
	}
	return row.toRunner(), nil
}

// ChatForSession resolves which chat a provider conversation belongs to, from
// APPEND-ONLY history — so it keeps answering long after the runner that opened
// the conversation has moved on or died. This is what makes a re-announced
// session id on /resume recognisable instead of looking like a brand-new
// conversation.
func (s *Store) ChatForSession(
	ctx context.Context,
	wsID string,
	sessionID string,
) (string, error) {
	var row conversationRow
	err := s.db.WithContext(ctx).
		Where("workspace_id = ? AND session_id = ?", wsID, sessionID).
		Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return "", fmt.Errorf("agentrunner store: chat for session %q: %w", sessionID, ErrNotFound)
	}
	if err != nil {
		return "", fmt.Errorf("agentrunner store: chat for session %q: %w", sessionID, err)
	}
	return row.ChatID, nil
}

// LastConversation returns the most recent conversation the chat has hosted —
// the one a resume picks up from. Ordered by first_seen_at, with SQLite's
// insertion-order rowid as the tiebreak, since two conversations opened by the
// same runner share that runner's StartedAt.
func (s *Store) LastConversation(
	ctx context.Context,
	chatID string,
) (domain.ChatConversation, error) {
	var row conversationRow
	err := s.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Order("first_seen_at DESC, rowid DESC").
		Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return domain.ChatConversation{}, fmt.Errorf("agentrunner store: last conversation for chat %q: %w", chatID, ErrNotFound)
	}
	if err != nil {
		return domain.ChatConversation{}, fmt.Errorf("agentrunner store: last conversation for chat %q: %w", chatID, err)
	}
	return row.toConversation(), nil
}

// Get returns the live runner with runnerID, or ErrNotFound once it has exited —
// the tombstone in the event log is audit, never liveness, so an exited runner is
// simply absent here.
func (s *Store) Get(
	ctx context.Context,
	runnerID string,
) (domain.AgentRunner, error) {
	var row runnerRow
	err := s.db.WithContext(ctx).Where("id = ?", runnerID).Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner store: get %q: %w", runnerID, ErrNotFound)
	}
	if err != nil {
		return domain.AgentRunner{}, fmt.Errorf("agentrunner store: get %q: %w", runnerID, err)
	}
	return row.toRunner(), nil
}

// AllLive returns every runner the read model believes is still running: the
// input to boot reconciliation, which asks the PTY — the sole authority — whether
// each one really is, and Exits the ones that are not.
//
// This is the one read that heals a lost read model (via whole-model Replay),
// because here an empty model is genuinely ambiguous: it means either "nothing is
// running" or "the read DB was lost". Getting that wrong at boot would strand a
// live CLI with no row pointing at it.
func (s *Store) AllLive(
	ctx context.Context,
) ([]domain.AgentRunner, error) {
	rows, err := s.healedRows(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]domain.AgentRunner, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toRunner())
	}
	return out, nil
}

// healedRows returns every live row, replaying the whole event log back into the
// read model first when the model is empty — the ambiguity AllLive documents.
func (s *Store) healedRows(
	ctx context.Context,
) ([]runnerRow, error) {
	rows, err := s.allRows(ctx)
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		return rows, nil
	}
	if err := s.rebuild(ctx); err != nil {
		return nil, err
	}
	return s.allRows(ctx)
}

// ForgetChat drops the chat's conversation history. It is the chat delete cascade
// — the ONLY thing permitted to remove append-only history, because a deleted
// chat is the one case where the history has nothing left to describe. It does
// not touch the live-runner model: a runner's row belongs to the runner's
// lifecycle (its PTY), not to any chat's.
func (s *Store) ForgetChat(
	ctx context.Context,
	chatID string,
) error {
	if err := s.db.WithContext(ctx).Delete(&conversationRow{}, "chat_id = ?", chatID).Error; err != nil {
		return fmt.Errorf("agentrunner store: forget chat %q: %w", chatID, err)
	}
	return nil
}

func (s *Store) allRows(
	ctx context.Context,
) ([]runnerRow, error) {
	var rows []runnerRow
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentrunner store: all live: %w", err)
	}
	return rows, nil
}

// rebuild enumerates every runner aggregate in the event log and Replays each
// back into the read model. Replaying an exited runner re-folds its exit, which
// deletes the live row again — so a rebuild reconstructs both projections
// exactly, including the absences. It is a no-op when the event store cannot
// enumerate its ids or the log is empty.
func (s *Store) rebuild(
	ctx context.Context,
) error {
	lister, ok := s.es.(serialize.AggregateLister)
	if !ok {
		return nil
	}
	keys, err := lister.AggregateIDs(ctx)
	if err != nil {
		return fmt.Errorf("agentrunner store: enumerate aggregate ids: %w", err)
	}
	fold := (&projector{db: s.db}).onEvent
	for _, key := range keys {
		id, found := strings.CutPrefix(key, eventKeyPrefix)
		if !found {
			continue
		}
		if err := s.ax.Replay(ctx, id, 1, 0, fold); err != nil {
			return fmt.Errorf("agentrunner store: replay %s: %w", id, err)
		}
	}
	return nil
}

// registerStoreProjection subscribes the read-model projection to every
// agentrunner event. It does not broadcast — hub.go owns fan-out — so the two
// derive independently from the same stream. Designed to register ONCE, on the
// singleton ax.
func registerStoreProjection(
	db *gormdb.DB,
	ax asynx.Asynx[domain.AgentRunner],
) error {
	p := &projector{db: db}
	if _, err := ax.Subscribe(asynx.Topic("agentrunner.*"), p.onEvent); err != nil {
		return fmt.Errorf("agentrunner store projection: subscribe: %w", err)
	}
	return nil
}

type projector struct {
	db *gormdb.DB
}

func (p *projector) onEvent(
	ctx context.Context,
	evt asynxModels.Event[domain.AgentRunner],
) {
	r := evt.Aggregate

	// EXITED: drop the live row. That deletion IS how the chat goes dormant —
	// there is no status to flip, so nothing is left behind that could later
	// disagree with the PTY. History (conversationRow) is untouched: a dormant
	// chat must stay resumable and its conversations recognisable on /resume.
	if r.ExitedAt != nil {
		if err := p.db.WithContext(ctx).Delete(&runnerRow{}, "id = ?", r.ID).Error; err != nil {
			slog.ErrorContext(ctx, "agentrunner projection: delete live row", "runner", r.ID, "err", err)
		}
		return
	}

	p.upsertLive(ctx, r)
	p.appendConversation(ctx, r)
}

// upsertLive writes the single live row for STARTED / SESSION_BOUND / MOVED.
// Placement is whatever the aggregate says — Crowbar is its only writer, so it
// cannot drift.
func (p *projector) upsertLive(
	ctx context.Context,
	r domain.AgentRunner,
) {
	row := runnerRow{
		ID:              r.ID,
		WorkspaceID:     r.WorkspaceID,
		ProviderID:      r.ProviderID,
		TerminalSession: r.TerminalSession,
		CurrentChatID:   r.CurrentChatID,
		CurrentSession:  r.CurrentSession,
		StartedAt:       r.StartedAt,
	}
	err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error
	if err != nil {
		slog.ErrorContext(ctx, "agentrunner projection: upsert live row", "runner", r.ID, "err", err)
	}
}

// appendConversation records the (chat, conversation) pair the runner now holds.
// Idempotent: DoNothing on conflict keeps the history append-only under replay,
// so a rebuilt model is byte-identical to the one it replaces.
func (p *projector) appendConversation(
	ctx context.Context,
	r domain.AgentRunner,
) {
	if r.CurrentSession == "" {
		return
	}
	conv := conversationRow{
		ChatID:      r.CurrentChatID,
		SessionID:   r.CurrentSession,
		WorkspaceID: r.WorkspaceID,
		ProviderID:  r.ProviderID,
		FirstSeenAt: r.StartedAt,
	}
	err := p.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&conv).Error
	if err != nil {
		slog.ErrorContext(ctx, "agentrunner projection: append conversation",
			"chat", r.CurrentChatID, "session", r.CurrentSession, "err", err)
	}
}
