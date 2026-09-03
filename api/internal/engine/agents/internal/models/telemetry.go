package models

import "time"

const (
	TelemetrySourceCallback = "callback"
	TelemetrySourceProbe    = "probe"
)

type Telemetry struct {
	ObservedAt time.Time
	Source     string

	Context    *ContextUsage
	RateLimits []RateLimitWindow
	Cost       *SessionCost
	Model      *ModelIdentity
}

func (t Telemetry) Empty() bool {
	return t.Context == nil && len(t.RateLimits) == 0 && t.Cost == nil && t.Model == nil
}

type ContextUsage struct {
	CapacityTokens   *int
	UsedTokens       *int
	UsedPercent      *float64
	RemainingPercent *float64
}

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
