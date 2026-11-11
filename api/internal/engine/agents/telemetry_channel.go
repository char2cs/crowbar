package agents

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

// TelemetryChannel is the channel this provider's usage reports arrive on: hooks
// for telemetry.callback (the relay POSTs it), "" for telemetry.probe (Crowbar runs
// it, so no channel owns it) or no mechanism, else the events.telemetry transport.
func (a *agent) TelemetryChannel() string {
	if t := a.spec.Telemetry; t != nil {
		if t.Callback != nil {
			return string(spec.ChannelHooks)
		}
		return "" // a probe Crowbar runs itself: no channel owns it
	}
	if _, declared := a.spec.Events[spec.HookTelemetry]; !declared {
		return ""
	}
	return a.spec.TransportFor(spec.HookTelemetry)
}

// TelemetryOnSurface reports whether a chat sitting on `surface` can receive
// this provider's usage reports, so the UI hides a gauge that could never move.
// A channel-less mechanism or an undeclared surface answers true.
func (a *agent) TelemetryOnSurface(surface string) bool {
	carries := a.TelemetryChannel()
	if carries == "" {
		return true
	}
	declared := a.SurfaceChannel(surface)
	if declared == "" {
		return true
	}
	return declared == carries
}
