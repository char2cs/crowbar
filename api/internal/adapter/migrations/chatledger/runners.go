package chatledger

import (
	"context"
	"database/sql"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// LiveRunners lists the runners the legacy runner store still holds on a
// chat. A missing database or table is none.
func LiveRunners(ctx context.Context, h home.Home) ([]LegacyRunner, error) {
	var out []LegacyRunner
	err := queryLegacy(ctx, filepath.Join(h.Store(), RunnerDB), "agent_runners", func(rows *sql.Rows) error {
		var r LegacyRunner
		if err := rows.Scan(&r.ID, &r.ChatID, &r.ProviderID); err != nil {
			return err
		}
		out = append(out, r)
		return nil
	}, `SELECT id, current_chat_id, provider_id FROM agent_runners
		WHERE current_chat_id != '' AND provider_id != '' ORDER BY started_at, id`)
	return out, err
}
