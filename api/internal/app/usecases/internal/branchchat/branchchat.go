// Package branchchat projects domain.Chat slices into domain.BranchChat slices
// for the branch review read model (09 §2). All functions are pure.
package branchchat

import (
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// From converts a flat list of chats into BranchChat projections relative to
// now. The returned slice is always non-nil.
func From(
	chats []domain.Chat,
	now time.Time,
) []domain.BranchChat {
	out := make([]domain.BranchChat, 0, len(chats))
	for _, c := range chats {
		out = append(out, domain.BranchChat{
			ID:       c.ID,
			Title:    c.Title,
			Age:      relativeAge(now, c.CreatedAt),
			IsActive: c.Status == domain.ChatStatusAgentRunning,
		})
	}
	return out
}

// relativeAge returns a human-readable relative-time string for the age of
// then relative to now. Buckets: <1m → "Xs", <1h → "Xm", <1d → "Xh", else "Xd".
func relativeAge(
	now time.Time,
	then time.Time,
) string {
	d := now.Sub(then)
	if d < 0 {
		d = 0
	}

	secs := int(d.Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}

	mins := int(d.Minutes())
	if mins < 60 {
		return fmt.Sprintf("%dm", mins)
	}

	hours := int(d.Hours())
	if hours < 24 {
		return fmt.Sprintf("%dh", hours)
	}

	days := int(d.Hours() / 24)
	return fmt.Sprintf("%dd", days)
}
