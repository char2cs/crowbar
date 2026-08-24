// Package protocol is how Crowbar speaks one provider's language.
//
// It is the single face over the three things that make a provider legible: reading
// its descriptor, translating its payloads in both directions, and (through the
// descriptor's transport declaration) knowing which wire carries them. Everything
// under internal/ here is reachable only through this file, so a caller learns one
// API instead of five packages.
//
// It holds no state and starts no processes. A conversation's lifecycle belongs to the
// runner; this only makes bytes meaningful.
package protocol

import (
	"context"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/answer"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/inbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/telemetry"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/termprompt"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

// Errors surfaced to callers, re-exported so nothing outside has to name the packages
// underneath.
var (
	ErrUnknownProvider   = descriptor.ErrUnknown
	ErrInvalidDescriptor = descriptor.ErrInvalid
	ErrUndeclaredEvent   = inbound.ErrUndeclaredEvent
	ErrForeignPayload    = inbound.ErrForeignConversation
	ErrUnsupportedFormat = inbound.ErrUnsupportedFormat
	ErrNotAnswerable     = answer.ErrNotAnswerable
	ErrUnsupportedAnswer = answer.ErrUnsupportedDecision
	ErrMalformedAnswer   = answer.ErrMalformedAnswer

	ErrTelemetryUnsupported    = telemetry.ErrUnsupported
	ErrTelemetryInvalidWorkdir = telemetry.ErrInvalidWorkdir
)

// ForeignPayloadError reports which required field was absent, i.e. why the payload
// was judged to describe another CLI's conversation.
type ForeignPayloadError = inbound.ForeignConversationError

// All returns every descriptor Crowbar can resolve, sorted by id.
func All(ctx context.Context, homeDir string) ([]*spec.Descriptor, error) {
	return descriptor.All(ctx, homeDir)
}

// Resolve loads one provider's descriptor, preferring an on-disk override.
func Resolve(ctx context.Context, homeDir, id string) (*spec.Descriptor, error) {
	return descriptor.Resolve(ctx, homeDir, id)
}

// Installed reports whether a provider's command exists on PATH.
func Installed(cmd string) bool { return descriptor.Installed(cmd) }

// CheckVersion refuses a provider whose protocol is outside the range its descriptor
// was written against.
func CheckVersion(d *spec.Descriptor, actual string) error {
	return descriptor.CheckProtocolVersion(d, actual)
}

// --- inbound: they tell us -------------------------------------------------

// Recv turns one raw provider payload into a canonical event.
func Recv(d *spec.Descriptor, canonical string, raw []byte) (models.CanonicalEvent, error) {
	return inbound.Parse(d, canonical, raw)
}

// Observes lists the canonical events this provider reports, sorted.
func Observes(d *spec.Descriptor) []string { return inbound.Declared(d) }

// RecvTelemetry maps the provider's own cost and capacity report.
func RecvTelemetry(d *spec.Descriptor, raw []byte, now time.Time) (models.Telemetry, error) {
	return telemetry.ParseCallback(d, raw, now)
}

// ProbeTelemetry runs the provider's telemetry probe, for providers that have no
// push channel.
func ProbeTelemetry(
	ctx context.Context, d *spec.Descriptor,
	opts models.ProbeOptions, acquire exec.Acquire, now time.Time,
) (models.Telemetry, error) {
	return telemetry.Probe(ctx, d, opts, acquire, now)
}

// --- outbound: we tell them ------------------------------------------------

// Send resolves a canonical outbound event into the provider's own call and payload.
// The bool is the capability check: a provider that declares no such event cannot be
// told to do it.
func Send(
	d *spec.Descriptor, canonical string, values map[string]string,
) (wireEvent string, payload map[string]string, ok bool) {
	return outbound.Resolve(d, canonical, values)
}

// Sends lists the canonical events Crowbar can drive on this provider.
func Sends(d *spec.Descriptor) []string { return outbound.Declared(d) }

// CanSend reports whether one outbound event is available.
func CanSend(d *spec.Descriptor, canonical string) bool {
	_, _, ok := outbound.Resolve(d, canonical, nil)
	return ok
}

// --- ask: they block on our reply ------------------------------------------

// AnswerCapability reports whether a decision can be sent back for this event, and
// which decisions the provider accepts.
func AnswerCapability(d *spec.Descriptor, canonical string) (models.AnswerCapability, bool) {
	return answer.Capability(d, canonical)
}

// Reply renders a human's decision into the bytes the provider expects.
func Reply(
	d *spec.Descriptor, canonical string, raw []byte, decision models.AnswerDecision,
) ([]byte, error) {
	return answer.Render(d, canonical, raw, decision)
}

// --- terminal: what only the screen can tell us ----------------------------

// TerminalPrompts reports whether the provider declares any screen-detected modal.
func TerminalPrompts(d *spec.Descriptor) bool { return termprompt.Declared(d) }

// MatchTerminalNotice matches a rendered screen against the provider's declared
// notices — a signpost to read, not a question to answer.
func MatchTerminalNotice(d *spec.Descriptor, screen string) (models.TerminalNotice, bool) {
	return termprompt.MatchNotice(d, screen)
}

// MatchTerminalPrompt matches a rendered screen against the provider's declared
// modals — the prompts that reach Crowbar through no hook at all.
func MatchTerminalPrompt(d *spec.Descriptor, screen string) (models.TerminalPrompt, bool) {
	return termprompt.Match(d, screen)
}
