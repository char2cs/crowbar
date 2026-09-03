package runner

import "context"

// seedPermissionLevel delegates to Conversations' own — SpawnChat and
// moveToNewChat are the two OTHER chat-creation paths, neither of which can
// call Conversations.MintChat itself (both pre-generate chatID before the
// row exists), but the seed itself is identical everywhere: the current
// global default, durably, best-effort.
func (rs *Runners) seedPermissionLevel(
	ctx context.Context,
	chatID string,
) {
	rs.conversations.SeedPermissionLevel(ctx, chatID)
}

// canonicalPermissionLevels is Crowbar's own guarded → trusted → full-auto
// ordering, least to most permissive — the ONLY place this feature encodes
// what "more permissive" means, and purely for resolvePermissionLevel's own
// use below. It names no provider vocabulary; these are Crowbar's own
// concept names, the same three every descriptor's permission_levels.levels
// map is keyed by.
var canonicalPermissionLevels = []string{"guarded", "trusted", "full-auto"}

// resolvePermissionLevel returns the level to actually spawn a provider
// under: desired unchanged if the provider declares it, otherwise the
// nearest canonical level AT LEAST as permissive as desired that the
// provider DOES declare — never a MORE restrictive substitute than what was
// asked for, since silently holding more than the user configured is the
// wrong direction to err in. If the provider declares nothing at or above
// desired, the least-restrictive level it declares at all; if it declares
// none whatsoever, desired unchanged (selection.Steps then simply finds no
// matching key and applies nothing, leaving the provider's own untouched
// default — never a spawn failure).
func resolvePermissionLevel(
	available []string,
	desired string,
) string {
	if containsLevel(available, desired) {
		return desired
	}
	start := 0
	for i, level := range canonicalPermissionLevels {
		if level == desired {
			start = i
			break
		}
	}
	for _, level := range canonicalPermissionLevels[start:] {
		if containsLevel(available, level) {
			return level
		}
	}
	for _, level := range canonicalPermissionLevels {
		if containsLevel(available, level) {
			return level
		}
	}
	return desired
}

func containsLevel(levels []string, want string) bool {
	for _, level := range levels {
		if level == want {
			return true
		}
	}
	return false
}
