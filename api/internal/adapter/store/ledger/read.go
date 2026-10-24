package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

// record is one chat_ledger row as the table holds it.
type record struct {
	ChatID string
	Seq    int64
	Kind   string
	TurnID string
	Scope  string
	At     int64
	Body   []byte
}

// Read returns chatID's rows after afterSeq in seq order, at most limit of
// them; limit <= 0 is all of them.
func (s *Store) Read(ctx context.Context, chatID string, afterSeq int64, limit int) ([]ledger.Row, error) {
	if limit <= 0 {
		limit = -1 // SQLite: no limit
	}
	var recs []record
	err := s.db.WithContext(ctx).Raw(
		`SELECT chat_id, seq, kind, turn_id, scope, at, body FROM chat_ledger
		 WHERE chat_id = ? AND seq > ? ORDER BY seq LIMIT ?`, chatID, afterSeq, limit,
	).Scan(&recs).Error
	if err != nil {
		return nil, fmt.Errorf("ledger: read %s: %w", chatID, err)
	}
	rows := make([]ledger.Row, 0, len(recs))
	for _, r := range recs {
		rows = append(rows, ledger.Row{
			ChatID: r.ChatID, Seq: r.Seq, Kind: ledger.Kind(r.Kind), TurnID: r.TurnID,
			Scope: r.Scope, At: fromMicros(r.At), Body: r.Body,
		})
	}
	return rows, nil
}

// OpenChats lists the chats whose last append left work unfinished.
func (s *Store) OpenChats(ctx context.Context) ([]string, error) {
	var ids []string
	err := s.db.WithContext(ctx).Raw(
		`SELECT chat_id FROM chat_ledger_heads WHERE open = 1 ORDER BY chat_id`,
	).Scan(&ids).Error
	if err != nil {
		return nil, fmt.Errorf("ledger: open chats: %w", err)
	}
	return ids, nil
}

// SessionOwner answers which chat a (provider, session) is bound to.
func (s *Store) SessionOwner(ctx context.Context, provider, sessionID string) (chatID string, ok bool, err error) {
	var owners []string
	err = s.db.WithContext(ctx).Raw(
		`SELECT chat_id FROM chat_ledger_sessions WHERE provider = ? AND session_id = ?`, provider, sessionID,
	).Scan(&owners).Error
	if err != nil {
		return "", false, fmt.Errorf("ledger: session owner %s %s: %w", provider, sessionID, err)
	}
	if len(owners) == 0 {
		return "", false, nil
	}
	return owners[0], true, nil
}

// BlobRefs are the payload refs chatID's tool calls carry.
func (s *Store) BlobRefs(ctx context.Context, chatID string) ([]string, error) {
	rows, err := s.Read(ctx, chatID, 0, 0)
	if err != nil {
		return nil, err
	}
	var refs []string
	for _, r := range rows {
		if r.Kind != ledger.KindToolCall {
			continue
		}
		body, err := ledger.DecodeBody[ledger.ToolCall](r)
		if err != nil {
			continue
		}
		for _, ref := range []string{body.InputRef, body.ResultRef} {
			if ref != "" {
				refs = append(refs, ref)
			}
		}
	}
	return refs, nil
}

// RefInUse reports whether any chat's tool call still carries ref.
func (s *Store) RefInUse(ctx context.Context, ref string) (bool, error) {
	var n int64
	err := s.db.WithContext(ctx).Raw(
		`SELECT COUNT(*) FROM chat_ledger WHERE kind = ? AND instr(body, ?) > 0`,
		string(ledger.KindToolCall), `"`+ref+`"`,
	).Scan(&n).Error
	if err != nil {
		return false, fmt.Errorf("ledger: ref in use %s: %w", ref, err)
	}
	return n > 0, nil
}

// RunnerStart is the runner_started row that put runnerID on a chat, and that
// chat; a purged chat's runners are gone with it.
func (s *Store) RunnerStart(ctx context.Context, runnerID string) (string, ledger.RunnerStarted, bool, error) {
	needle, err := json.Marshal(runnerID)
	if err != nil {
		return "", ledger.RunnerStarted{}, false, fmt.Errorf("ledger: runner start %s: %w", runnerID, err)
	}
	var recs []record
	err = s.db.WithContext(ctx).Raw(
		`SELECT chat_id, kind, body FROM chat_ledger WHERE kind = ? AND instr(body, ?) > 0 ORDER BY seq`,
		string(ledger.KindRunnerStarted), `"runnerId":`+string(needle),
	).Scan(&recs).Error
	if err != nil {
		return "", ledger.RunnerStarted{}, false, fmt.Errorf("ledger: runner start %s: %w", runnerID, err)
	}
	for _, r := range recs {
		body, err := ledger.DecodeBody[ledger.RunnerStarted](ledger.Row{Kind: ledger.Kind(r.Kind), Body: r.Body})
		if err == nil && body.RunnerID == runnerID {
			return r.ChatID, body, true, nil
		}
	}
	return "", ledger.RunnerStarted{}, false, nil
}

// fromMicros is the time a stored at column names, as the store stamped it.
func fromMicros(us int64) time.Time { return time.UnixMicro(us).UTC() }
