package chatledger

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

// LiveRunners lists the runners the legacy runner store still holds on a
// chat. A missing database or table is none.
func LiveRunners(ctx context.Context, h home.Home) ([]LegacyRunner, error) {
	var out []LegacyRunner
	err := queryLegacy(ctx, filepath.Join(h.Store(), RunnerDB), "agent_runners", func(rows *sql.Rows) error {
		c, err := scanCells(rows, 4)
		if err != nil {
			return err
		}
		r := LegacyRunner{ID: c.str(), ChatID: c.str(), ProviderID: c.str(), StartedAt: c.time("started_at")}
		// The start only dates the runner's row, so one it cannot read is none.
		if c.err != nil {
			r.StartedAt = time.Time{}
		}
		out = append(out, r)
		return nil
	}, func(map[string]bool) string {
		return `SELECT COALESCE(id, ''), current_chat_id, provider_id, started_at FROM agent_runners
		WHERE current_chat_id != '' AND provider_id != '' ORDER BY started_at, id`
	})
	return out, err
}
