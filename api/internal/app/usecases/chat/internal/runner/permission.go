package runner

import (
	"slices"

	"github.com/char2cs/crowbar/api/internal/domain"
)

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
