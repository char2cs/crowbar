package agents

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

func (a *agent) TelemetryChannel() string {
	if t := a.spec.Telemetry; t != nil {
		if t.Callback != nil {
			return string(spec.ChannelHooks)
		}
		return ""
	}
	if _, declared := a.spec.Events[spec.HookTelemetry]; !declared {
		return ""
	}
	return a.spec.TransportFor(spec.HookTelemetry)
}

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
