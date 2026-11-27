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

const LegacyNodeLog = "node.db"

const legacyChatLog = "agent_chat.db"

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
