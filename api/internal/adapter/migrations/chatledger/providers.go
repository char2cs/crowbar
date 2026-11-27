package chatledger

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

type legacySource struct {
	db, table string
	cols      legacyTable
	where     string
	args      []any
}

var sources = []legacySource{
	{RunnerDB, "agent_chat_conversations", columns(text("chat_id"), text("provider_id"), added(bare("last_active_at"))), "", nil},
	{ActivityDB, "agent_interruptions", columns(text("chat_id"), text("detail"), bare("at")), "WHERE kind = ?", []any{providerSwitched}},
	{RunnerDB, "agent_chat_placements", columns(text("chat_id"), text("provider_id"), bare("last_placed_at")), "", nil},
}

func columns(cols ...legacyCol) legacyTable { return legacyTable{cols: cols} }

func (src legacySource) query(has map[string]bool) string {
	return "SELECT " + src.cols.selectList(has) + " FROM " + src.table + " " + src.where + " ORDER BY 3, 2"
}

func NewestProviders(ctx context.Context, h home.Home) (map[string]string, error) {
	newest := map[string]placed{}
	for _, src := range sources {
		err := queryLegacy(ctx, filepath.Join(h.Store(), src.db), src.table, func(rows *sql.Rows) error {
			c, err := scanCells(rows, 3)
			if err != nil {
				return err
			}
			p := placed{ChatID: c.str(), ProviderID: c.str(), At: c.time(src.cols.cols[2].name)}
			if c.err != nil {
				return c.err
			}
			if seen, ok := newest[p.ChatID]; !ok || p.newerThan(seen) {
				newest[p.ChatID] = p
			}
			return nil
		}, src.query, src.args...)
		if err != nil {
			return nil, err
		}
	}
	out := make(map[string]string, len(newest))
	for chatID, p := range newest {
		out[chatID] = p.ProviderID
	}
	return out, nil
}

func (p placed) newerThan(seen placed) bool {
	return p.At.After(seen.At) || p.At.Equal(seen.At) && p.ProviderID < seen.ProviderID
}
