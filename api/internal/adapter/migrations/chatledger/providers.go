package chatledger

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// legacySource is one table that once said which provider ran on a chat.
type legacySource struct {
	db, table, query string
	args             []any
}

// sources are read in this order; a later row wins only when strictly newer.
var sources = []legacySource{
	{
		RunnerDB, "agent_chat_conversations",
		"SELECT chat_id, provider_id, last_active_at FROM agent_chat_conversations", nil,
	},
	{
		ActivityDB, "agent_interruptions",
		"SELECT chat_id, detail, at FROM agent_interruptions WHERE kind = ?",
		[]any{providerSwitched},
	},
	{
		RunnerDB, "agent_chat_placements",
		"SELECT chat_id, provider_id, last_placed_at FROM agent_chat_placements", nil,
	},
}

// NewestProviders maps each chat the legacy history names to the provider it
// names newest: among the chat's conversations, its provider switches and its
// placements. Missing databases or tables are empty history.
func NewestProviders(ctx context.Context, h home.Home) (map[string]string, error) {
	newest := map[string]placed{}
	for _, src := range sources {
		rows, err := readSource(ctx, filepath.Join(h.Store(), src.db), src)
		if err != nil {
			return nil, err
		}
		for _, p := range rows {
			if seen, ok := newest[p.ChatID]; !ok || p.At.After(seen.At) {
				newest[p.ChatID] = p
			}
		}
	}
	out := make(map[string]string, len(newest))
	for chatID, p := range newest {
		out[chatID] = p.ProviderID
	}
	return out, nil
}

func readSource(ctx context.Context, path string, src legacySource) (out []placed, err error) {
	db, ok, err := openLegacy(path)
	if err != nil || !ok {
		return nil, err
	}
	defer func() { err = errors.Join(err, db.Close()) }()
	var n int
	if err := db.QueryRowContext(ctx,
		"SELECT count(*) FROM sqlite_master WHERE type = 'table' AND name = ?", src.table).Scan(&n); err != nil {
		return nil, fmt.Errorf("chatledger: %s: %w", src.table, err)
	}
	if n == 0 {
		return nil, nil
	}
	rows, err := db.QueryContext(ctx, src.query, src.args...)
	if err != nil {
		return nil, fmt.Errorf("chatledger: %s: %w", src.table, err)
	}
	return scanPlaced(rows, src.table)
}

func scanPlaced(rows *sql.Rows, table string) (out []placed, err error) {
	defer func() { err = errors.Join(err, rows.Close()) }()
	for rows.Next() {
		var p placed
		if err := rows.Scan(&p.ChatID, &p.ProviderID, &p.At); err != nil {
			return nil, fmt.Errorf("chatledger: %s: %w", table, err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("chatledger: %s: %w", table, err)
	}
	return out, nil
}
