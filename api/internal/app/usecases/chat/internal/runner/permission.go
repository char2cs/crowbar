package runner

import (
	"context"
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// seedPermissionLevel seeds the global default for the creation paths (SpawnChat,
// moveToNewChat) that cannot go through Conversations.MintChat.
func (rs *Runners) seedPermissionLevel(
	ctx context.Context,
	chatID string,
) {
	rs.conversations.SeedPermissionLevel(ctx, chatID)
}

// resolvePermissionLevel returns desired if declared, else the nearest declared level
// AT LEAST as permissive (never more restrictive than configured), else the most
// permissive declared; with none declared, desired unchanged (no step applies).
func resolvePermissionLevel(
	available []string,
	desired string,
) string {
	if slices.Contains(available, desired) {
		return desired
	}
	canonicalPermissionLevels := domain.PermissionLevels()
	start := 0
	for i, level := range canonicalPermissionLevels {
		if level == desired {
			start = i
			break
		}
	}
	for _, level := range canonicalPermissionLevels[start:] {
		if slices.Contains(available, level) {
			return level
		}
	}
	for _, level := range canonicalPermissionLevels {
		if slices.Contains(available, level) {
			return level
		}
	}
	return desired
}
