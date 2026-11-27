package agents

import (
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/catalog"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/exec"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/protocol"
	"github.com/char2cs/crowbar/api/internal/engine/agents/internal/spawn"
)

var (
	ErrUnknownAgent = protocol.ErrUnknownProvider

	ErrInvalidDescriptor = protocol.ErrInvalidDescriptor

	ErrForbiddenFlag = spawn.ErrForbiddenFlag

	ErrPromptSubmitUnsupported = errPromptSubmitUnsupported

	ErrHookUnsupportedFormat = protocol.ErrUnsupportedFormat

	ErrHookUndeclared = protocol.ErrUndeclaredEvent

	ErrForeignConversation = protocol.ErrForeignPayload

	ErrHookVariantMismatch = protocol.ErrVariantMismatch

	ErrRequiredFieldMissing = protocol.ErrRequiredFieldMissing

	ErrTelemetryUnsupported = protocol.ErrTelemetryUnsupported

	ErrTelemetryInvalidWorkdir = protocol.ErrTelemetryInvalidWorkdir

	ErrCatalogUnsupported = catalog.ErrUnsupported

	ErrCatalogInvalidWorkdir = catalog.ErrInvalidWorkdir

	ErrCatalogMalformedOutput = catalog.ErrMalformedOutput

	ErrProbeTimeout = catalog.ErrTimeout

	ErrProbeOutputLimit = exec.ErrOutputLimit

	ErrProbeCommandUnavailable = exec.ErrCommandUnavailable

	ErrProbeCommandFailed = exec.ErrCommandFailed

	ErrNotAnswerable = protocol.ErrNotAnswerable

	ErrUnsupportedDecision = protocol.ErrUnsupportedAnswer

	ErrMalformedAnswer = protocol.ErrMalformedAnswer

	ErrAPITransportNotDeclared = errAPITransportNotDeclared

	ErrAPISessionLost = protocol.ErrAPISessionLost
)

func APIFailureReason(err error) (string, bool) { return protocol.APIFailureReason(err) }
