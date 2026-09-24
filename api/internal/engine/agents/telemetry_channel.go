package agents

import "github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"

// TelemetryChannel is the delivery CHANNEL this provider's usage reports
// arrive on — "" when it declares no mechanism at all, or one that belongs to
// no channel in particular.
//
// Three shapes, and the answer follows the mechanism rather than any provider
// name:
//
//   - telemetry.callback is a payload the hook relay POSTs (claude wires it as
//     a statusLine command), so it is the HOOKS channel by construction.
//   - telemetry.probe is a command CROWBAR runs, so it belongs to no channel
//     and reaches a chat on any surface — "" says exactly that.
//   - an `events.telemetry` entry is an ordinary inbound event, and its
//     transport is the channel that carries it (codex: thread/tokenUsage/
//     updated over the api transport).
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
// this provider's usage reports at all.
//
// The house rule is absence, never a dead control (context-gauge.tsx's own
// doc): a gauge drawn over a channel that carries no usage is a number that
// can never move, and a STALE one once the chat has been somewhere that did.
//
// Absence is not a decision, twice over — a provider whose mechanism belongs
// to no channel, and a surface this descriptor does not declare, both answer
// true, so nothing that predates surfaces loses a gauge it had.
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
