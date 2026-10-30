package migrations

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/migrate"
)

// LegacyNodeLog is the retired position aggregate's event log under state/events.
const LegacyNodeLog = "node.db"

// planPlacementTree opens no store: a home with the retired node log has
// placements to copy, how many is only known once the stores are open.
func planPlacementTree(_ context.Context, env migrate.Env) (migrate.Plan, error) {
	_, err := os.Stat(filepath.Join(env.Home.Events(), LegacyNodeLog))
	if errors.Is(err, fs.ErrNotExist) {
		return migrate.Plan{}, nil
	}
	if err != nil {
		return migrate.Plan{}, fmt.Errorf("placement-tree: plan: %w", err)
	}
	return migrate.Plan{
		Actions:       []migrate.Action{{Kind: migrate.Convert, Subject: "sidebar placements → project trees", Count: migrate.Unknown}},
		RewritesState: true,
	}, nil
}
