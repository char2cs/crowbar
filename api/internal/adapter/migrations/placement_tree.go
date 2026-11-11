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

// legacyChatLog is the chat event log, whose older events carry each placement.
const legacyChatLog = "agent_chat.db"

// planPlacementTree opens no store: a home with either log has placements to
// copy, how many is only known once the stores are open.
func planPlacementTree(_ context.Context, env migrate.Env) (migrate.Plan, error) {
	found := false
	for _, name := range []string{LegacyNodeLog, legacyChatLog} {
		_, err := os.Stat(filepath.Join(env.Home.Events(), name))
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			return migrate.Plan{}, fmt.Errorf("placement-tree: plan: %w", err)
		}
		found = true
	}
	if !found {
		return migrate.Plan{}, nil
	}
	return migrate.Plan{
		Actions:       []migrate.Action{{Kind: migrate.Convert, Subject: "sidebar placements → project trees", Count: migrate.Unknown}},
		RewritesState: true,
	}, nil
}
