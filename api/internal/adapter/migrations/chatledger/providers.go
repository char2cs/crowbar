package chatledger

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// legacySource is one table that once said which provider ran on a chat.
type legacySource struct {
	db, table, query string
	args             []any
}

var sources = []legacySource{
	{
		RunnerDB, "agent_chat_conversations",
		"SELECT chat_id, provider_id, last_active_at FROM agent_chat_conversations ORDER BY last_active_at, provider_id",
		nil,
	},
	{
		ActivityDB, "agent_interruptions",
		"SELECT chat_id, detail, at FROM agent_interruptions WHERE kind = ? ORDER BY at, detail",
		[]any{providerSwitched},
	},
	{
		RunnerDB, "agent_chat_placements",
		"SELECT chat_id, provider_id, last_placed_at FROM agent_chat_placements ORDER BY last_placed_at, provider_id",
		nil,
	},
}

// NewestProviders maps each chat the legacy history names to the provider it
// names newest among its conversations, provider switches and placements; a
// tie goes to the lowest provider id. Missing databases or tables are empty.
func NewestProviders(ctx context.Context, h home.Home) (map[string]string, error) {
	newest := map[string]placed{}
	for _, src := range sources {
		err := queryLegacy(ctx, filepath.Join(h.Store(), src.db), src.table, func(rows *sql.Rows) error {
			var p placed
			if err := rows.Scan(&p.ChatID, &p.ProviderID, &p.At); err != nil {
				return err
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
