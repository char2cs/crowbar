package protocol

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/models"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/apidriver"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/jsonrpc"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/answer"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/inbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/outbound"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/telemetry"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol/internal/translate/termprompt"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spec"
)

var (
	ErrUnknownProvider      = descriptor.ErrUnknown
	ErrInvalidDescriptor    = descriptor.ErrInvalid
	ErrUndeclaredEvent      = inbound.ErrUndeclaredEvent
	ErrForeignPayload       = inbound.ErrForeignConversation
	ErrVariantMismatch      = inbound.ErrVariantMismatch
	ErrRequiredFieldMissing = inbound.ErrRequiredFieldMissing
	ErrUnsupportedFormat    = inbound.ErrUnsupportedFormat
	ErrNotAnswerable        = answer.ErrNotAnswerable
	ErrUnsupportedAnswer    = answer.ErrUnsupportedDecision
	ErrMalformedAnswer      = answer.ErrMalformedAnswer

	ErrTelemetryUnsupported    = telemetry.ErrUnsupported
	ErrTelemetryInvalidWorkdir = telemetry.ErrInvalidWorkdir

	ErrAPISessionLost = apidriver.ErrSessionLost
)

func APIFailureReason(err error) (string, bool) {
	var call *jsonrpc.CallError
	if !errors.As(err, &call) {
		return "", false
	}
	if call.Detail == "" {
		return call.Message, true
	}
	return call.Message + ": " + call.Detail, true
}

type ForeignPayloadError = inbound.ForeignConversationError

type RequiredFieldError = inbound.RequiredFieldError

type DescriptorRuleFailure = descriptor.RuleFailure

type DescriptorSource = descriptor.Source

func CheckDescriptor(raw []byte) (*spec.Descriptor, []DescriptorRuleFailure) {
	return descriptor.Check(raw)
}

func CheckDescriptorVersion(raw []byte) error {
	return descriptor.CheckVersion(raw)
}

func UnknownDescriptorKeys(raw []byte) []string {
	return descriptor.UnknownKeys(raw)
}

func DescriptorSourceFor(homeDir, id string) (DescriptorSource, bool) {
	return descriptor.SourceFor(homeDir, id)
}

func EmbeddedDescriptorSource(id string) (DescriptorSource, bool) {
	return descriptor.EmbeddedSource(id)
}

func DescriptorSources(homeDir string) ([]DescriptorSource, error) {
	return descriptor.Sources(homeDir)
}

func All(ctx context.Context, homeDir string, accept func(raw []byte) bool) ([]*spec.Descriptor, error) {
	return descriptor.All(ctx, homeDir, accept)
}

func EmbeddedModelManifest() []byte {
	return descriptor.EmbeddedModelManifest()
}

func Resolve(ctx context.Context, homeDir, id string, accept func(raw []byte) bool) (*spec.Descriptor, error) {
	return descriptor.Resolve(ctx, homeDir, id, accept)
}

func OverridePath(homeDir, id string) string {
	return descriptor.OverridePath(homeDir, id)
}

func Installed(cmd string) bool { return descriptor.Installed(cmd) }

func CheckVersion(d *spec.Descriptor, actual string) error {
	return descriptor.CheckProtocolVersion(d, actual)
}

func Recv(d *spec.Descriptor, canonical string, raw []byte, channel spec.Channel) (models.CanonicalEvent, error) {
	return inbound.Parse(d, canonical, raw, channel)
}

func Observes(d *spec.Descriptor) []string { return inbound.Declared(d) }

func RecvTelemetry(
	d *spec.Descriptor, raw []byte, now time.Time, channel spec.Channel,
) (models.Telemetry, error) {
	return telemetry.ParseCallback(d, raw, now, channel)
}

func ProbeTelemetry(
	ctx context.Context, d *spec.Descriptor,
	opts models.ProbeOptions, acquire exec.Acquire, now time.Time,
) (models.Telemetry, error) {
	return telemetry.Probe(ctx, d, opts, acquire, now)
}

func Send(
	d *spec.Descriptor, canonical string, values map[string]string,
) (wireEvent string, payload map[string]string, ok bool) {
	return outbound.Resolve(d, canonical, values)
}

func CanSend(d *spec.Descriptor, canonical string) bool {
	_, _, ok := outbound.Resolve(d, canonical, nil)
	return ok
}

func AnswerCapability(d *spec.Descriptor, canonical string) (models.AnswerCapability, bool) {
	return answer.Capability(d, canonical)
}

func Reply(
	d *spec.Descriptor, canonical string, raw []byte, decision models.AnswerDecision,
) ([]byte, error) {
	return answer.Render(d, canonical, raw, decision)
}

func TerminalPrompts(d *spec.Descriptor) bool { return termprompt.Declared(d) }

func MatchTerminalNotice(d *spec.Descriptor, screen string) (models.TerminalNotice, bool) {
	return termprompt.MatchNotice(d, screen)
}

func MatchTerminalPrompt(d *spec.Descriptor, screen string) (models.TerminalPrompt, bool) {
	return termprompt.Match(d, screen)
}

type APIEvent = apidriver.Event

type APISessionOrigin = apidriver.SessionOrigin

type APIStdio = jsonrpc.Stdio

type APIConn struct{ drv *apidriver.Driver }

func (c *APIConn) Events() <-chan APIEvent { return c.drv.Events() }

func (c *APIConn) Reply(askID json.RawMessage, rendered []byte) error {
	return c.drv.Reply(askID, rendered)
}

func (c *APIConn) Close() error { return c.drv.Close() }

func (c *APIConn) Overflowed() bool { return c.drv.Overflowed() }

func (c *APIConn) EstablishSession(
	ctx context.Context, canonical string, values map[string]string,
) (map[string]string, error) {
	return c.drv.EstablishSession(ctx, canonical, values)
}

func (c *APIConn) Dispatch(
	ctx context.Context, canonical string, values map[string]string,
) (map[string]string, error) {
	return c.drv.Dispatch(ctx, canonical, values)
}

func (c *APIConn) Send(ctx context.Context, canonical string, values map[string]string) error {
	return c.drv.Send(ctx, canonical, values)
}

func (c *APIConn) InjectAt(ctx context.Context, at string, values map[string]string) error {
	return c.drv.InjectAt(ctx, at, values)
}

func StartAPIDriver(
	ctx context.Context, d *spec.Descriptor, stdio APIStdio, origin APISessionOrigin,
) (*APIConn, error) {
	drv, err := apidriver.Start(ctx, d, stdio, origin)
	if err != nil {
		return nil, err
	}
	return &APIConn{drv: drv}, nil
}
