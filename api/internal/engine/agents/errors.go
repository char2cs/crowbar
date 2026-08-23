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

var (
	ErrUnknownAgent = descriptor.ErrUnknown

	ErrInvalidDescriptor = descriptor.ErrInvalid

	ErrForbiddenFlag = spawn.ErrForbiddenFlag

	ErrPromptSubmitUnsupported = errPromptSubmitUnsupported

	ErrHookUnsupportedFormat = hooks.ErrUnsupportedFormat

	ErrHookUndeclared = hooks.ErrUndeclaredEvent

	ErrForeignConversation = hooks.ErrForeignConversation

	ErrTelemetryUnsupported = telemetry.ErrUnsupported

	ErrTelemetryInvalidWorkdir = telemetry.ErrInvalidWorkdir

	ErrCatalogUnsupported = catalog.ErrUnsupported

	ErrCatalogInvalidWorkdir = catalog.ErrInvalidWorkdir

	ErrCatalogMalformedOutput = catalog.ErrMalformedOutput

	ErrProbeTimeout = catalog.ErrTimeout

	ErrProbeOutputLimit = exec.ErrOutputLimit

	ErrProbeCommandUnavailable = exec.ErrCommandUnavailable

	ErrProbeCommandFailed = exec.ErrCommandFailed

	ErrNotAnswerable = answers.ErrNotAnswerable

	ErrUnsupportedDecision = answers.ErrUnsupportedDecision

	ErrMalformedAnswer = answers.ErrMalformedAnswer
)
