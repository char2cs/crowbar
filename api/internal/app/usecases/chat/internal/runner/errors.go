package runner

import (
	"fmt"

	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
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

// ErrStopped is returned by a parked switch or resume when the user pressed Stop:
// Stop preempts rather than queues behind it, and the request changed nothing.
var ErrStopped = fmt.Errorf("agent: the chat was stopped while this request waited: %w", problem.ErrConflict)

// ErrProviderExitedDuringStartup is returned when the vendor CLI died before
// runner_started committed: an interactive CLI that exits at once has not started.
// It wraps problem.ErrFailedDependency (424) — something the user can fix.
var ErrProviderExitedDuringStartup = fmt.Errorf(
	"agent: spawn runner: provider process exited during startup: %w", problem.ErrFailedDependency,
)

// ErrSpawnSessionRefused is a spawn whose provider refused the session it
// resumes. That session is lost, and the chat's next launch starts a fresh one.
var ErrSpawnSessionRefused = &problem.Error{
	Code: problem.SessionLost, Message: "agent: spawn runner: the provider refused the session this chat resumes",
	Err: problem.ErrConflict,
}

// errRefusedInStartup is a CLI that died in startup refusing the session it
// resumed: recorded as any startup exit is, answered as the lost session it is
// rather than as a failed dependency.
type errRefusedInStartup struct{}

func (errRefusedInStartup) Error() string {
	return ErrSpawnSessionRefused.Message + ": provider process exited during startup"
}

func (errRefusedInStartup) Unwrap() error { return ErrSpawnSessionRefused }

func (errRefusedInStartup) Is(target error) bool { return target == ErrProviderExitedDuringStartup }

// ErrNoLiveRunner is a chat with no CLI on it: dormant, the normal state
// between runs, so a reader treats it as an answer rather than a failure.
var ErrNoLiveRunner = problem.New(problem.NotFound, "agent: chat has no live runner")

// ErrChatHasLiveRunner refuses a second CLI on a chat: one runner per chat is
// the live table's key.
var ErrChatHasLiveRunner = fmt.Errorf("agent: chat already has a live runner: %w", problem.ErrConflict)

// ErrChatProviderUnknown is returned when a dormant chat records no provider to
// resume on, so the client asks which to start. It keeps the 404.
var ErrChatProviderUnknown = fmt.Errorf(
	"agent: this chat no longer records which provider it ran: %w", problem.ErrNotFound,
)

var (
	// ErrPromptBusy means the chat is working, or holds a prompt the provider has
	// not acknowledged yet.
	ErrPromptBusy = &problem.Error{Code: problem.ChatBusy, Message: "agent: chat is busy", Err: problem.ErrConflict}

	// ErrPromptRequestIDConflict means one clientRequestId was reused for different
	// prompt text, or for a prompt already settled. Reusing an id can never
	// silently submit a different operation.
	ErrPromptRequestIDConflict = &problem.Error{Code: problem.RequestIDConflict, Message: "agent: client request id was already used for a different prompt", Err: problem.ErrConflict}

	// ErrPromptOutcomeUnknown means a CLI may have received the prompt but Crowbar
	// cannot confirm it did. Retrying automatically could duplicate a prompt the
	// provider accepted.
	ErrPromptOutcomeUnknown = &problem.Error{Code: problem.RequestOutcomeUncertain, Message: "agent: prior prompt delivery outcome is uncertain; inspect the chat before retrying with a new request id", Err: problem.ErrConflict}

	// ErrPromptWithProvider means a retract came after the CLI was handed the
	// prompt: only stopping its turn takes it back now.
	ErrPromptWithProvider = &problem.Error{Code: problem.PromptWithProvider, Message: "the prompt is already with its provider", Err: problem.ErrConflict}

	// ErrPromptSessionLost means the launch carrying the prompt was refused the
	// session it resumed: the provider never took the prompt.
	ErrPromptSessionLost = &problem.Error{Code: problem.SessionLost, Message: "agent: the provider no longer has this chat's session", Err: problem.ErrConflict}

	// ErrPromptUnsupported leaves the native terminal fully usable while telling
	// React that this provider has no declarative restart-TUI submission mapping.
	ErrPromptUnsupported = &problem.Error{Code: problem.PromptSubmitUnsupported, Message: "agent: provider does not support React prompt submission", Err: problem.ErrUnprocessable}

	// ErrSpawnPromptUndeliverable means a spawn's user message reached neither carrier
	// (PTY argv or api Dispatch). Failing, never reporting success, keeps the text in
	// the client's queue once the caller settles the prompt.
	ErrSpawnPromptUndeliverable = fmt.Errorf("agent: spawn runner: the prompt this spawn carries reached no carrier: %w", problem.ErrBadGateway)

	// ErrPromptSessionUnavailable means there is no live native TUI to replace.
	// A live lazy TUI need not have announced a session yet; that is a safe fresh
	// submission, not this error.
	ErrPromptSessionUnavailable = &problem.Error{Code: problem.LiveTUIRequired, Message: "agent: chat has no live provider TUI for prompt submission", Err: problem.ErrUnprocessable}
)
