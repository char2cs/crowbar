package runner

import (
	"errors"
	"fmt"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

var (
	ErrSlashCatalogUnsupported = &problem.Error{Code: problem.CatalogUnsupported, Message: "agent: provider does not support deterministic slash catalog discovery", Err: problem.ErrUnprocessable}
	ErrSlashCatalogTimeout     = &problem.Error{Code: problem.CatalogTimeout, Message: "agent: slash catalog discovery timed out", Err: problem.ErrTimeout}
	ErrSlashCatalogUnavailable = &problem.Error{Code: problem.CatalogCommandUnavailable, Message: "agent: provider command for slash catalog discovery is unavailable", Err: engineterminal.ErrCommandNotFound}
	ErrSlashCatalogOutputLimit = &problem.Error{Code: problem.CatalogOutputLimit, Message: "agent: slash catalog command exceeded its safe output limit", Err: problem.ErrBadGateway}
	ErrSlashCatalogCommand     = &problem.Error{Code: problem.CatalogCommandFailed, Message: "agent: slash catalog provider command failed", Err: problem.ErrBadGateway}
	ErrSlashCatalogMalformed   = &problem.Error{Code: problem.CatalogMalformedOutput, Message: "agent: slash catalog provider output was malformed", Err: problem.ErrBadGateway}
	ErrSlashCatalogSuperseded  = &problem.Error{Code: problem.CatalogSuperseded, Message: "agent: slash catalog request was superseded by a newer request", Err: problem.ErrConflict}
)

var ErrStopped = fmt.Errorf("agent: the chat was stopped while this request waited: %w", problem.ErrConflict)

var ErrProviderExitedDuringStartup = fmt.Errorf(
	"agent: spawn runner: provider process exited during startup: %w", problem.ErrFailedDependency,
)

var ErrSpawnSessionRefused = &problem.Error{
	Code: problem.SessionLost, Message: "agent: spawn runner: the provider refused the session this chat resumes",
	Err: problem.ErrConflict,
}

type errRefusedInStartup struct{}

func (errRefusedInStartup) Error() string {
	return ErrSpawnSessionRefused.Message + ": provider process exited during startup"
}

func (errRefusedInStartup) Unwrap() error { return ErrSpawnSessionRefused }

func (errRefusedInStartup) Is(target error) bool { return target == ErrProviderExitedDuringStartup }

var ErrNoLiveRunner = problem.New(problem.NotFound, "agent: chat has no live runner")

var ErrChatHasLiveRunner = fmt.Errorf("agent: chat already has a live runner: %w", problem.ErrConflict)

var ErrChatProviderUnknown = fmt.Errorf(
	"agent: this chat no longer records which provider it ran: %w", problem.ErrNotFound,
)

var (
	ErrPromptBusy = &problem.Error{Code: problem.ChatBusy, Message: "agent: chat is busy", Err: problem.ErrConflict}

	ErrPromptRequestIDConflict = &problem.Error{Code: problem.RequestIDConflict, Message: "agent: client request id was already used for a different prompt", Err: problem.ErrConflict}

	ErrPromptOutcomeUnknown = &problem.Error{Code: problem.RequestOutcomeUncertain, Message: "agent: prior prompt delivery outcome is uncertain; inspect the chat before retrying with a new request id", Err: problem.ErrConflict}

	ErrPromptWithProvider = &problem.Error{Code: problem.PromptWithProvider, Message: "the prompt is already with its provider", Err: problem.ErrConflict}

	ErrPromptSessionLost = &problem.Error{Code: problem.SessionLost, Message: "agent: the provider no longer has this chat's session", Err: problem.ErrConflict}

	ErrPromptUnsupported = &problem.Error{Code: problem.PromptSubmitUnsupported, Message: "agent: provider does not support React prompt submission", Err: problem.ErrUnprocessable}

	ErrSpawnPromptUndeliverable = fmt.Errorf("agent: spawn runner: the prompt this spawn carries reached no carrier: %w", problem.ErrBadGateway)

	ErrPromptSessionUnavailable = &problem.Error{Code: problem.LiveTUIRequired, Message: "agent: chat has no live provider TUI for prompt submission", Err: problem.ErrUnprocessable}
)

type connectFailure struct{ err error }

func (e *connectFailure) Error() string { return "agent: spawn runner: api connect: " + e.err.Error() }

func (e *connectFailure) Unwrap() error { return e.err }

func failureDetail(err error) string {
	if reason, ok := engineagents.APIFailureReason(err); ok {
		return reason
	}
	var cf *connectFailure
	if errors.As(err, &cf) {
		return cf.err.Error()
	}
	return err.Error()
}
