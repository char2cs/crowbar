package dto

import (
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents"
)

// AgentTelemetryDTO is what the provider itself reported about cost and capacity.
//
// Every field is a POINTER because "not reported" and "zero" are different facts
// and a gauge rendering 0% for an unreported value is a lie. A provider that
// reports nothing yields an absent section, and the client draws no gauge.
type AgentTelemetryDTO struct {
	ObservedAt time.Time              `json:"observedAt"`
	Source     string                 `json:"source"`
	Context    *AgentContextUsageDTO  `json:"context,omitempty"`
	RateLimits []AgentRateLimitDTO    `json:"rateLimits,omitempty"`
	Cost       *AgentSessionCostDTO   `json:"cost,omitempty"`
	Model      *AgentModelIdentityDTO `json:"model,omitempty"`
}

// AgentTelemetryDTOFrom maps the provider's usage report onto the wire — the one
// conversion the GET and the pushed `telemetry` frame share.
func AgentTelemetryDTOFrom(report agents.Telemetry) AgentTelemetryDTO {
	out := AgentTelemetryDTO{ObservedAt: report.ObservedAt, Source: report.Source}
	if c := report.Context; c != nil {
		out.Context = &AgentContextUsageDTO{
			CapacityTokens: c.CapacityTokens, UsedTokens: c.UsedTokens,
			UsedPercent: c.UsedPercent, RemainingPercent: c.RemainingPercent,
		}
	}
	for _, w := range report.RateLimits {
		out.RateLimits = append(out.RateLimits, AgentRateLimitDTO{
			ID: w.ID, Label: w.Label, UsedPercent: w.UsedPercent, ResetsAt: w.ResetsAt,
		})
	}
	if c := report.Cost; c != nil {
		out.Cost = &AgentSessionCostDTO{TotalUSD: c.TotalUSD, APIDurationMS: c.APIDurationMS}
	}
	if m := report.Model; m != nil {
		out.Model = &AgentModelIdentityDTO{ID: m.ID, DisplayName: m.DisplayName}
	}
	return out
}

type AgentContextUsageDTO struct {
	CapacityTokens   *int     `json:"capacityTokens,omitempty"`
	UsedTokens       *int     `json:"usedTokens,omitempty"`
	UsedPercent      *float64 `json:"usedPercent,omitempty"`
	RemainingPercent *float64 `json:"remainingPercent,omitempty"`
}

type AgentRateLimitDTO struct {
	ID          string     `json:"id"`
	Label       string     `json:"label,omitempty"`
	UsedPercent *float64   `json:"usedPercent,omitempty"`
	ResetsAt    *time.Time `json:"resetsAt,omitempty"`
}

type AgentSessionCostDTO struct {
	TotalUSD      *float64 `json:"totalUsd,omitempty"`
	APIDurationMS *int     `json:"apiDurationMs,omitempty"`
}

type AgentModelIdentityDTO struct {
	ID          string `json:"id"`
	DisplayName string `json:"displayName,omitempty"`
}
