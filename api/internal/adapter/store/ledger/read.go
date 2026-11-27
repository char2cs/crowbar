package ledger

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain/ledger"
)

type record struct {
	ChatID string
	Seq    int64
	Kind   string
	TurnID string
	Scope  string
	At     int64
	Body   []byte
}

func (s *Store) Read(ctx context.Context, chatID string, afterSeq int64, limit int) (_ []ledger.Row, err error) {
	if limit <= 0 {
		limit = -1
	}
	cur, err := s.db.WithContext(ctx).Raw(
		`SELECT chat_id, seq, kind, turn_id, scope, at, body FROM chat_ledger
		 WHERE chat_id = ? AND seq > ? ORDER BY seq LIMIT ?`, chatID, afterSeq, limit,
	).Rows()
	if err != nil {
		return nil, fmt.Errorf("ledger: read %s: %w", chatID, err)
	}
	defer func() {
		if closeErr := cur.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("ledger: read %s: close: %w", chatID, closeErr)
		}
	}()
	rows := make([]ledger.Row, 0)
	for cur.Next() {
		var r record
		if err := cur.Scan(&r.ChatID, &r.Seq, &r.Kind, &r.TurnID, &r.Scope, &r.At, &r.Body); err != nil {
			return nil, fmt.Errorf("ledger: read %s: %w: %w", chatID, ledger.ErrUnreadableRow, err)
		}
		rows = append(rows, ledger.Row{
			ChatID: r.ChatID, Seq: r.Seq, Kind: ledger.Kind(r.Kind), TurnID: r.TurnID,
			Scope: r.Scope, At: fromMicros(r.At), Body: r.Body,
		})
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("ledger: read %s: %w", chatID, err)
	}
	return rows, nil
}

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

func (s *Store) RefsInUse(ctx context.Context, refs []string) (inUse map[string]bool, err error) {
	inUse = make(map[string]bool, len(refs))
	if len(refs) == 0 {
		return inUse, nil
	}
	want := make(map[string]bool, len(refs))
	for _, ref := range refs {
		if ref != "" {
			want[ref] = true
		}
	}
	cur, err := s.db.WithContext(ctx).Raw(
		`SELECT body FROM chat_ledger WHERE kind = ?`, string(ledger.KindToolCall),
	).Rows()
	if err != nil {
		return nil, fmt.Errorf("ledger: refs in use: %w", err)
	}
	defer func() {
		if closeErr := cur.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("ledger: refs in use: close: %w", closeErr)
		}
	}()
	for cur.Next() {
		if ctx.Err() != nil {
			return inUse, fmt.Errorf("ledger: refs in use: %w", ctx.Err())
		}
		var body []byte
		if err := cur.Scan(&body); err != nil {
			continue
		}
		tc, err := ledger.DecodeBody[ledger.ToolCall](ledger.Row{Kind: ledger.KindToolCall, Body: body})
		if err != nil {
			continue
		}
		if want[tc.InputRef] {
			inUse[tc.InputRef] = true
		}
		if want[tc.ResultRef] {
			inUse[tc.ResultRef] = true
		}
	}
	if err := cur.Err(); err != nil {
		return nil, fmt.Errorf("ledger: refs in use: %w", err)
	}
	return inUse, nil
}

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

func fromMicros(us int64) time.Time { return time.UnixMicro(us).UTC() }
