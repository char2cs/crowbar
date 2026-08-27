package models

// RiskTier classifies how much latitude a tool call needs, independent of
// which provider raised it. Each provider's descriptor maps its own tool
// vocabulary into this fixed, provider-blind scale (see EventSpec.Risk); Go
// never inspects a raw tool name to decide it.
type RiskTier string

const (
	// RiskReadOnly inspects, changes nothing.
	RiskReadOnly RiskTier = "read-only"
	// RiskStandard is an ordinary edit or command inside the workspace.
	RiskStandard RiskTier = "standard"
	// RiskSensitive is destructive, external-facing, or — critically — simply
	// unclassified. It is the safe default: a tool the descriptor's risk table
	// doesn't name gets the most conservative tier, never the most permissive.
	RiskSensitive RiskTier = "sensitive"
	// RiskInternal marks a call to Crowbar's own injected tool surface. It is
	// not part of the guarded/trusted/full-auto scale at all — see
	// permission.AutoApprove — because no human is present in a Crowbar-driven
	// pane to answer for it; holding it would only ever stall it.
	RiskInternal RiskTier = "internal"
)
