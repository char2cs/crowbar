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

// ErrStopped is returned by a switch or resume that was parked (on the
// outgoing turn, or on a prompt delivery) when the user pressed Stop. Stop
// preempts the parked request rather than queueing behind it (invariant A2),
// and the preempted request has changed nothing.
var ErrStopped = fmt.Errorf("agent: the chat was stopped while this request waited: %w", problem.ErrConflict)

// ErrProviderExitedDuringStartup is returned when a provider's vendor CLI died
// before its runner_started row could commit.
//
// Crowbar hosts the ordinary INTERACTIVE CLI in a real PTY — the engine's hardest
// constraint, asserted by every descriptor's spawn.interactive_required — so a
// process that exits on the spot has not started, whatever its exit code. Handing
// the caller a chat with a corpse behind it would be a chat whose pane attaches to
// a PTY that is already gone.
//
// It wraps problem.ErrFailedDependency (424), the same class as a vendor CLI that
// is not installed at all: the request was well-formed and the daemon is healthy,
// and what failed is a dependency the user can act on — an expired login, a broken
// install, a CLI that refuses this workspace.
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

// ErrChatProviderUnknown is returned when a dormant chat cannot be resumed
// because the chat records no provider. It is the PROVIDER that is missing, not
// a runner (a dormant chat has none by definition), so the client asks which
// provider to start instead of picking one. It keeps the 404.
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

	// ErrSpawnPromptUndeliverable means a spawn was handed a user message and
	// could not get it to any carrier. A spawn carries a prompt in exactly one
	// of two places — the forked PTY's own argv, or a Dispatch down the api
	// connection it adopted INSTEAD of forking one — and a PTY-less spawn has
	// only the second. Failing here is the point: reporting success with the
	// message nowhere is what silently lost a user's prompt (see
	// carryPromptOverAPIConn), and the caller settles the prompt, which keeps the
	// text in the client's own queue.
	ErrSpawnPromptUndeliverable = fmt.Errorf("agent: spawn runner: the prompt this spawn carries reached no carrier: %w", problem.ErrBadGateway)

	// ErrPromptSessionUnavailable means there is no live native TUI to replace.
	// A live lazy TUI need not have announced a session yet; that is a safe fresh
	// submission, not this error.
	ErrPromptSessionUnavailable = &problem.Error{Code: problem.LiveTUIRequired, Message: "agent: chat has no live provider TUI for prompt submission", Err: problem.ErrUnprocessable}
)
