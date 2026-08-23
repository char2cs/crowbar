// Package store owns the agentrunner read side: TWO projections folded from the
// AgentRunner event stream, which together replace everything AgentSegment used
// to be asked. They are different in KIND, and the difference is the whole point.
//
//  1. agent_runners (runnerRow) — LIVE rows. One row per RUNNING CLI, deleted on
//     exit. "Does a row exist for this chat" IS the liveness question: there is
//     no status column, so there is nothing to go stale. This is the core rule of
//     the refactor — we persist PLACEMENT (which chat, which conversation), never
//     LIVENESS. Liveness belongs to the PTY alone. Two authorities on liveness
//     always drift, and that drift is the bug being deleted here (today a segment
//     can read "ended" while its CLI is demonstrably still running).
//
//     This table is therefore NEVER rebuilt from the event log — see heal.go. A
//     PTY does not survive a daemon restart, so every runner the log remembers is
//     already dead by the time anything could replay it; resurrecting one would
//     manufacture exactly the "live row, dead CLI" state this package exists to
//     make unrepresentable. An empty agent_runners is the truth, not a symptom.
//
//  2. agent_chat_conversations (conversationRow) — APPEND-ONLY history. Every
//     conversation a chat has ever hosted. Never updated, never deleted except by
//     the chat delete cascade (ForgetChat). It survives the process, so a dormant
//     chat stays resumable and its old conversations stay recognisable on a later
//     /resume. Append-only history cannot drift from reality; only live state can
//     — which is precisely why THIS one is durable truth worth healing, and is
//     rebuilt from the log (once, at construction) when it is lost.
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

	"github.com/char2cs/crowbar/api/internal/engine/agents"

	"github.com/char2cs/asynx"
	asynxModels "github.com/char2cs/asynx/models"
	gormdb "gorm.io/gorm"
	"gorm.io/gorm/clause"
)

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
}

// New builds both read models over db, heals the conversation history if it was
// lost (heal.go — history only, never the live-runner table), then registers the
// read-model projection (this file) and the hub broadcast projection (hub.go) on
// ax.
//
// es is the same per-type event log ax wraps; the pair is used ONCE here, for
// that construction-time heal, and never again from a read path. No read heals:
// for the live table an empty model is the normal steady state of an idle
// machine, so healing on a read would replay a monotonically-growing event log on
// every dormant lookup — and would resurrect dead runners while doing it. The
// heal itself fires on a MISSING MARKER, not on an empty table, so a user who
// deletes their last chat does not get its history resurrected on the next boot
// (heal.go).
//
// broadcast must not be nil: the hub projection calls it on every event, so a nil
// one panics inside a projection goroutine, far from whoever built the Store.
func New(
	db *gormdb.DB,
	es asynxModels.Store,
	ax asynx.Asynx[agents.Runner],
	watch WatchFunc,
) (*Store, error) {
	if watch == nil {
		return nil, fmt.Errorf("agentrunner store: nil watch")
	}
	if err := db.AutoMigrate(&runnerRow{}, &conversationRow{}, &healMarkerRow{}); err != nil {
		return nil, fmt.Errorf("agentrunner store: migrate: %w", err)
	}
	if err := healConversations(db, es, ax); err != nil {
		return nil, err
	}
	if err := registerStoreProjection(db, ax); err != nil {
		return nil, fmt.Errorf("agentrunner store: projections: %w", err)
	}
	if err := registerHubProjection(ax, watch); err != nil {
		return nil, fmt.Errorf("agentrunner store: projections: %w", err)
	}
	return &Store{db: db}, nil
}

// newestArrivalFirst orders live runners by WHEN THEY ARRIVED where they are — the later
// of "when this process started" and "when it took its current conversation" — newest
// first, with the id as a final deterministic tiebreak.
//
// It is LOAD-BEARING on the eviction path, and a backstop everywhere else. Both halves of
// that sentence matter, so read them together with agentrunner.Displace's doc.
//
// Invariants I2/I3 are upheld primarily by DISPLACEMENT: Crowbar cannot kill a process
// synchronously, so whenever it takes a CLI off a chat it records that placement fact at
// once instead of waiting for the corpse to fall over, and a displaced runner points at
// nothing and cannot be returned by these queries at all. On the switch and delete paths
// that displace happens BEFORE any successor is spawned, so no two-candidate window exists.
//
// An EVICTION is different, and this ordering is what makes it come out right: the mover's
// Move is recorded FIRST (reality is not negotiable — spec §3) and the incumbent is
// displaced immediately after, so for that instant BOTH are placed on the chat. The one
// that arrived last is the mover, which is exactly who holds it. Deleting this ordering
// would hand out the evictee — a corpse — about half the time.
//
// What an ordering could NOT do, and why displacement exists at all: the outgoing CLI of a
// provider switch may not have announced its conversation yet, and when it finally does —
// AFTER the incoming CLI has started — it stamps a fresher timestamp than the incoming
// runner's spawn, so "whoever arrived last" would hand the chat straight back to the
// corpse. Timestamps say when things happened; only a displacement records what we DECIDED.
const newestArrivalFirst = "MAX(current_session_since, started_at) DESC, id ASC"

// LiveRunnerForChat returns the runner currently pointed at chatID, or ErrNotFound when
// the chat is dormant. There is no status column to consult: the row exists exactly while
// its CLI runs, so its presence IS liveness — and a runner Crowbar has taken OFF this chat
// (Displace) points at nothing and is not a candidate, even while its process is still
// dying.
func (s *Store) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (agents.Runner, error) {
	// "" is NOWHERE, not a chat. A displaced runner's row carries an empty chat id, so
	// without this an empty key would MATCH those rows and hand a caller back a runner that
	// is on nothing — the read model volunteering a lie, which is the shape this whole
	// model exists to delete.
	if chatID == "" {
		return agents.Runner{}, fmt.Errorf("agentrunner store: live runner for chat: %w", ErrNotFound)
	}
	var row runnerRow
	err := s.db.WithContext(ctx).
		Where("current_chat_id = ?", chatID).
		Order(newestArrivalFirst).
		Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return agents.Runner{}, fmt.Errorf("agentrunner store: live runner for chat %q: %w", chatID, ErrNotFound)
	}
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner store: live runner for chat %q: %w", chatID, err)
	}
	return row.toRunner(), nil
}

// LiveRunnersForChat returns EVERY live runner placed on chatID, newest arrival first.
//
// It is the read that makes I2 an invariant rather than a coincidence. LiveRunnerForChat
// answers "who holds this chat" and is what serving paths want; this one answers "who is
// ON this chat", which is the question the two PLACEMENT paths must ask — because their job
// is to leave exactly one runner there, and a single-row read cannot tell "nobody else" from
// "somebody else, and maybe more".
//
// Both placement paths call it immediately after their write commits (both are SendWait, so
// the read sees it) and retire everyone but themselves. In the ordinary case it returns one
// row — the caller itself — and nothing happens. It earns its keep only in the windows
// where a CLI arrives on a chat between another one's decision to take it and its arrival:
// a hook (never gated, never may be) moving a runner onto a chat a gated switch is mid-FORK
// of, which is a window as wide as a process spawn.
//
// An empty chatID is NOWHERE, and nowhere holds nobody.
func (s *Store) LiveRunnersForChat(
	ctx context.Context,
	chatID string,
) ([]agents.Runner, error) {
	if chatID == "" {
		return nil, nil
	}
	var rows []runnerRow
	err := s.db.WithContext(ctx).
		Where("current_chat_id = ?", chatID).
		Order(newestArrivalFirst).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentrunner store: live runners for chat %q: %w", chatID, err)
	}
	out := make([]agents.Runner, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toRunner())
	}
	return out, nil
}

// LiveRunnersForSession returns EVERY live runner holding conversation sessionID, newest
// arrival first — the I3 twin of LiveRunnersForChat, and it exists for the same reason.
//
// A placement onto a conversation (a bind, a move) must leave exactly ONE runner holding it,
// because two CLIs on one provider session id both write the same session file and corrupt
// it. And the single-row read cannot serve that: by the time the write has committed, the
// caller IS the newest holder, so LiveRunnerForSession would hand it back its own row and it
// would evict nobody at all.
//
// An empty sessionID is NOWHERE, and nowhere is held by nobody.
func (s *Store) LiveRunnersForSession(
	ctx context.Context,
	wsID string,
	sessionID string,
) ([]agents.Runner, error) {
	if sessionID == "" {
		return nil, nil
	}
	var rows []runnerRow
	err := s.db.WithContext(ctx).
		Where("workspace_id = ? AND current_session = ?", wsID, sessionID).
		Order(newestArrivalFirst).
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentrunner store: live runners for session %q: %w", sessionID, err)
	}
	out := make([]agents.Runner, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toRunner())
	}
	return out, nil
}

// LiveRunnerForSession returns the runner currently holding conversation sessionID in
// wsID — the incumbent the eviction path must displace (invariant I3: at most one live
// runner per conversation, because two CLIs on one provider session id both write the
// same session file). ErrNotFound means nobody is running that conversation right now,
// which is not the same as "it never existed" — that question is ChatForSession's, and
// it is answered from history.
//
// Ordered newest-arrival-first for the same reason as LiveRunnerForChat: if a runner is
// ALREADY being evicted off this conversation, the incumbent a second mover must
// displace is the one that took it over, not the corpse.
func (s *Store) LiveRunnerForSession(
	ctx context.Context,
	wsID string,
	sessionID string,
) (agents.Runner, error) {
	// "" is NOWHERE — see LiveRunnerForChat. A runner that has been displaced, or that has
	// not announced a conversation yet, holds no session, and must never be matched by one.
	if sessionID == "" {
		return agents.Runner{}, fmt.Errorf("agentrunner store: live runner for session: %w", ErrNotFound)
	}
	var row runnerRow
	err := s.db.WithContext(ctx).
		Where("workspace_id = ? AND current_session = ?", wsID, sessionID).
		Order(newestArrivalFirst).
		Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return agents.Runner{}, fmt.Errorf("agentrunner store: live runner for session %q: %w", sessionID, ErrNotFound)
	}
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner store: live runner for session %q: %w", sessionID, err)
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
// the one a resume picks up from. Ordered by first_seen_at, which is when the
// CONVERSATION opened (never when its runner spawned), so a chat that two runners
// have written into still orders by which conversation is newer. SQLite's
// insertion-order rowid breaks an exact tie, keeping the answer deterministic
// when two conversations share a timestamp.
func (s *Store) LastConversation(
	ctx context.Context,
	chatID string,
) (agents.ChatConversation, error) {
	var row conversationRow
	err := s.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Order("first_seen_at DESC, rowid DESC").
		Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return agents.ChatConversation{}, fmt.Errorf("agentrunner store: last conversation for chat %q: %w", chatID, ErrNotFound)
	}
	if err != nil {
		return agents.ChatConversation{}, fmt.Errorf("agentrunner store: last conversation for chat %q: %w", chatID, err)
	}
	return row.toConversation(), nil
}

// ConversationsForChat returns every conversation the chat has hosted, OLDEST
// FIRST — the same ordering LastConversation reads backwards, so "the newest
// conversation for provider X" is the last match when scanning forward.
//
// A provider switch needs this and cannot use LastConversation: after a handoff
// the chat's newest conversation belongs to the provider being switched away
// FROM, while the one being switched TO must be resumed into the conversation it
// left behind here — an older row.
func (s *Store) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]agents.ChatConversation, error) {
	var rows []conversationRow
	err := s.db.WithContext(ctx).
		Where("chat_id = ?", chatID).
		Order("first_seen_at ASC, rowid ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("agentrunner store: conversations for chat %q: %w", chatID, err)
	}
	out := make([]agents.ChatConversation, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toConversation())
	}
	return out, nil
}

// Get returns the live runner with runnerID, or ErrNotFound once it has exited —
// the tombstone in the event log is audit, never liveness, so an exited runner is
// simply absent here.
func (s *Store) Get(
	ctx context.Context,
	runnerID string,
) (agents.Runner, error) {
	var row runnerRow
	err := s.db.WithContext(ctx).Where("id = ?", runnerID).Take(&row).Error
	if errors.Is(err, gormdb.ErrRecordNotFound) {
		return agents.Runner{}, fmt.Errorf("agentrunner store: get %q: %w", runnerID, ErrNotFound)
	}
	if err != nil {
		return agents.Runner{}, fmt.Errorf("agentrunner store: get %q: %w", runnerID, err)
	}
	return row.toRunner(), nil
}

// AllLive returns every runner the read model believes is still running: the
// input to boot reconciliation, which asks the PTY — the sole authority — whether
// each one really is, and Exits the ones that are not.
//
// It reads the table and nothing else. An empty result is a real answer ("nothing
// is running"), which on an idle machine is the normal one: the live table is not
// durable truth to be recovered, it is a cache of what the PTYs are doing right
// now, and after a restart the honest answer is always "nothing".
func (s *Store) AllLive(
	ctx context.Context,
) ([]agents.Runner, error) {
	var rows []runnerRow
	if err := s.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("agentrunner store: all live: %w", err)
	}
	out := make([]agents.Runner, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.toRunner())
	}
	return out, nil
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

// registerStoreProjection subscribes the read-model projection to every
// agentrunner event. It does not broadcast — hub.go owns fan-out — so the two
// derive independently from the same stream. Designed to register ONCE, on the
// singleton ax.
func registerStoreProjection(
	db *gormdb.DB,
	ax asynx.Asynx[agents.Runner],
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
	evt asynxModels.Event[agents.Runner],
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
	if err := appendConversation(ctx, p.db, r); err != nil {
		slog.ErrorContext(ctx, "agentrunner projection: append conversation",
			"chat", r.CurrentChatID, "session", r.CurrentSession, "err", err)
	}
}

// upsertLive writes the single live row for STARTED / SESSION_BOUND / MOVED.
// Placement is whatever the aggregate says — Crowbar is its only writer, so it
// cannot drift.
func (p *projector) upsertLive(
	ctx context.Context,
	r agents.Runner,
) {
	row := runnerRow{
		ID:                      r.ID,
		WorkspaceID:             r.WorkspaceID,
		ProviderID:              r.ProviderID,
		TerminalSession:         r.TerminalSession,
		CurrentChatID:           r.CurrentChatID,
		CurrentSession:          r.CurrentSession,
		CurrentSessionSince:     r.CurrentSessionSince,
		LaunchSessionID:         r.LaunchSessionID,
		LaunchModel:             r.LaunchModel,
		LaunchEffort:            r.LaunchEffort,
		CurrentSessionResumable: r.CurrentSessionResumable,
		StartedAt:               r.StartedAt,
	}
	err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		UpdateAll: true,
	}).Create(&row).Error
	if err != nil {
		slog.ErrorContext(ctx, "agentrunner projection: upsert live row", "runner", r.ID, "err", err)
	}
}

// appendConversation records the (chat, conversation) pair the runner now holds,
// stamped with when that conversation OPENED (CurrentSessionSince — the moment it
// was bound or moved into), never with the runner's spawn time. Idempotent:
// DoNothing on conflict keeps the history append-only under replay, so a rebuilt
// model is identical to the one it replaces.
//
// Package-level rather than a projector method because the boot heal folds it
// WITHOUT the live-row writer (heal.go) — history is the only thing a replay is
// allowed to touch.
//
// It RETURNS its write error rather than logging it, because the two folds must
// treat a failure differently: the live projection logs and carries on (a
// projection must never take the daemon down), while the heal must refuse to mark
// the read model built (heal.go) — a heal that lost rows is a failed heal.
func appendConversation(
	ctx context.Context,
	db *gormdb.DB,
	r agents.Runner,
) error {
	if r.CurrentSession == "" {
		return nil
	}
	conv := conversationRow{
		ChatID:      r.CurrentChatID,
		SessionID:   r.CurrentSession,
		WorkspaceID: r.WorkspaceID,
		ProviderID:  r.ProviderID,
		FirstSeenAt: r.CurrentSessionSince,
	}
	err := db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&conv).Error
	if err != nil {
		return fmt.Errorf("agentrunner store: append conversation (chat %q, session %q): %w",
			r.CurrentChatID, r.CurrentSession, err)
	}
	return nil
}
