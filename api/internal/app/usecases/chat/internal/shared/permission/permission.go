// Package permission owns the per-chat trust dial that decides how much of a
// provider's own tool-call approval Crowbar answers automatically instead of
// holding for a human.
package permission

import engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"

// Level is how far up the RiskTier scale a chat auto-resolves prompts.
type Level string

const (
	// Guarded holds every prompt for a human — today's behavior, unchanged.
	Guarded Level = "guarded"
	// Trusted auto-resolves read-only and standard tiers; sensitive still holds.
	Trusted Level = "trusted"
	// FullAuto auto-resolves every tier, no exceptions.
	FullAuto Level = "full-auto"
)

// AutoApprove reports whether a chat at level should auto-resolve a prompt of
// risk, instead of holding it for a human.
//
// RiskInternal always auto-resolves, independent of level — it marks a call
// to Crowbar's own injected tool surface, which runs with no human present in
// the pane to answer for it. Holding it would not make it safer, only stall
// it forever, so it sits outside the guarded/trusted/full-auto dial entirely.
func AutoApprove(
	level Level,
	risk engineagents.RiskTier,
) bool {
	if risk == engineagents.RiskInternal {
		return true
	}
	switch level {
	case FullAuto:
		return true
	case Trusted:
		return risk == engineagents.RiskReadOnly || risk == engineagents.RiskStandard
	default:
		return false
	}
}
