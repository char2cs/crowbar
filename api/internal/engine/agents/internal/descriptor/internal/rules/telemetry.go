package rules

import (
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

type telemetry struct{}

func (telemetry) Name() string { return "telemetry" }

var knownFacts = map[string]struct{}{
	spec.FactContextCapacityTokens: {},
	spec.FactContextUsedTokens:     {},
	spec.FactContextUsedPercent:    {},
	spec.FactContextRemainPercent:  {},
	spec.FactCostTotalUSD:          {},
	spec.FactCostAPIDurationMS:     {},
	spec.FactModelID:               {},
	spec.FactModelDisplayName:      {},
}

func (r telemetry) Check(d *spec.Descriptor) error {
	t := d.Telemetry
	if t == nil {
		return nil
	}
	if t.Callback == nil && t.Probe == nil {
		return invalid(d.ID, "telemetry declares no transport")
	}
	if err := r.checkCallback(d, t.Callback); err != nil {
		return err
	}
	return r.checkProbe(d, t.Probe)
}

func (r telemetry) checkCallback(d *spec.Descriptor, cb *spec.TelemetryCallbackSpec) error {
	if cb == nil {
		return nil
	}
	if cb.Format != "json" {
		return invalid(d.ID, "telemetry.callback has unsupported format %q", cb.Format)
	}
	if err := r.checkFields(d.ID, "callback", cb.Fields); err != nil {
		return err
	}
	return r.checkRateLimits(d.ID, cb.RateLimits)
}

func (r telemetry) checkProbe(d *spec.Descriptor, probe *spec.TelemetryProbeSpec) error {
	if probe == nil {
		return nil
	}
	if probe.Format != "json" {
		return invalid(d.ID, "telemetry.probe has unsupported format %q", probe.Format)
	}
	if len(probe.Command) == 0 || hasEmptyArg(probe.Command) {
		return invalid(d.ID, "telemetry.probe.command must be fixed non-empty argv")
	}
	if strings.ContainsAny(strings.Join(probe.Command, ""), "{}") {
		return invalid(d.ID, "telemetry.probe.command must be fixed argv")
	}
	if flag, found := forbidden(probe.Command, d.Spawn.ForbidFlags); found {
		return invalid(d.ID, "telemetry.probe.command contains forbidden flag %q", flag)
	}
	return r.checkFields(d.ID, "probe", probe.Fields)
}

func (telemetry) checkFields(id, transport string, fields map[string]string) error {
	if len(fields) == 0 {
		return invalid(id, "telemetry.%s maps no fields", transport)
	}
	for fact, path := range fields {
		if _, ok := knownFacts[fact]; !ok {
			return invalid(id, "telemetry.%s maps unknown fact %q", transport, fact)
		}
		if path == "" {
			return invalid(id, "telemetry.%s fact %q has an empty path", transport, fact)
		}
	}
	return nil
}

func (telemetry) checkRateLimits(id string, windows []spec.TelemetryRateLimitMap) error {
	seen := map[string]struct{}{}
	for _, w := range windows {
		if w.ID == "" {
			return invalid(id, "telemetry.callback.rate_limits entry has no id")
		}
		if _, dup := seen[w.ID]; dup {
			return invalid(id, "telemetry.callback.rate_limits has duplicate id %q", w.ID)
		}
		seen[w.ID] = struct{}{}
		if w.UsedPercent == "" && w.ResetsAt == "" {
			return invalid(id, "telemetry.callback.rate_limits %q maps nothing", w.ID)
		}
	}
	return nil
}
