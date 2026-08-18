package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/answers"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/descriptor"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/hooks"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/telemetry"
)

// Every sentinel the engine can return. They are re-exported from the package
// that raises them so a caller matches one name and never imports below this
// root.
var (
	// ErrUnknownAgent reports a provider id no descriptor answers to.
	ErrUnknownAgent = descriptor.ErrUnknown
	// ErrInvalidDescriptor reports a descriptor that failed validation.
	ErrInvalidDescriptor = descriptor.ErrInvalid
	// ErrForbiddenFlag reports an assembled argv that would make the CLI headless.
	ErrForbiddenFlag = spawn.ErrForbiddenFlag

	// ErrPromptSubmitUnsupported reports a provider with no chat-side prompt
	// delivery. It is terminal-only for that operation, which is a capability
	// statement rather than a failure.
	ErrPromptSubmitUnsupported = errPromptSubmitUnsupported

	// ErrHookUnsupportedFormat reports a declared payload encoding the engine
	// cannot read.
	ErrHookUnsupportedFormat = hooks.ErrUnsupportedFormat
	// ErrHookUndeclared reports a canonical kind this descriptor does not map.
	ErrHookUndeclared = hooks.ErrUndeclaredEvent
	// ErrForeignConversation reports a hook that is not this CLI's own
	// user-facing conversation and must be dropped.
	ErrForeignConversation = hooks.ErrForeignConversation

	// ErrTelemetryUnsupported reports a provider that declares no telemetry.
	ErrTelemetryUnsupported = telemetry.ErrUnsupported
	// ErrTelemetryInvalidWorkdir reports a telemetry probe with no valid worktree.
	ErrTelemetryInvalidWorkdir = telemetry.ErrInvalidWorkdir

	// ErrCatalogUnsupported reports a provider that declares no slash catalogue.
	ErrCatalogUnsupported = catalog.ErrUnsupported
	// ErrCatalogInvalidWorkdir reports a missing or relative working directory.
	ErrCatalogInvalidWorkdir = catalog.ErrInvalidWorkdir
	// ErrCatalogMalformedOutput reports provider output the adapter cannot read.
	ErrCatalogMalformedOutput = catalog.ErrMalformedOutput

	// ErrProbeTimeout reports a provider command exceeding its declared budget.
	ErrProbeTimeout = catalog.ErrTimeout
	// ErrProbeOutputLimit reports a provider command writing past its ceiling.
	ErrProbeOutputLimit = exec.ErrOutputLimit
	// ErrProbeCommandUnavailable reports a provider executable that is not
	// installed.
	ErrProbeCommandUnavailable = exec.ErrCommandUnavailable
	// ErrProbeCommandFailed reports a provider command exiting non-zero.
	ErrProbeCommandFailed = exec.ErrCommandFailed

	// ErrNotAnswerable reports a provider that declares no way to answer an event's
	// prompt. It is a capability statement rather than a failure: the caller falls
	// through and the CLI puts up its own dialog.
	ErrNotAnswerable = answers.ErrNotAnswerable
	// ErrUnsupportedDecision reports a decision this provider has no template for.
	// Refused rather than approximated.
	ErrUnsupportedDecision = answers.ErrUnsupportedDecision
	// ErrMalformedAnswer reports a rendered answer that is not valid JSON, which
	// can only mean a mis-authored descriptor.
	ErrMalformedAnswer = answers.ErrMalformedAnswer
)
