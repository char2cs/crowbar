package models

import "time"

// Telemetry ingress sources.
const (
	TelemetrySourceCallback = "callback"
	TelemetrySourceProbe    = "probe"
)

// Telemetry is what a provider reported about the session's cost and capacity at
// one moment. Every fact is independently optional, and nothing is derived that
// was not reported: a percentage is computed only when capacity and usage are
// both known. A wrong gauge is worse than no gauge.
//
// It is CURRENT STATE, not history. Thousands of "19% used" observations exist
// only to be superseded, so telemetry does not enter the event log; only notable
// transitions do.
type Telemetry struct {
	ObservedAt time.Time
	Source     string

	Context    *ContextUsage
	RateLimits []RateLimitWindow
	Cost       *SessionCost
	Model      *ModelIdentity
}

// Empty reports whether the provider gave us nothing at all, in which case there
// is nothing to store and nothing to render.
func (t Telemetry) Empty() bool {
	return t.Context == nil && len(t.RateLimits) == 0 && t.Cost == nil && t.Model == nil
}

// ContextUsage is how much of the model's context the session has consumed.
// Pointers throughout, because "not reported" and "zero" are different facts and
// a gauge that renders 0% for an unreported value is a lie.
type ContextUsage struct {
	CapacityTokens   *int
	UsedTokens       *int
	UsedPercent      *float64
	RemainingPercent *float64
}

// RateLimitWindow is one provider-declared usage window. Providers disagree on
// how many exist and what they are called, so each is declared by the descriptor
// rather than assumed by the engine.
type RateLimitWindow struct {
	ID          string
	Label       string
	UsedPercent *float64
	ResetsAt    *time.Time
}

type SessionCost struct {
	TotalUSD      *float64
	APIDurationMS *int
}

type ModelIdentity struct {
	ID          string
	DisplayName string
}
