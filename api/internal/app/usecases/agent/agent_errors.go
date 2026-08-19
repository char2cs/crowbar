package agent

import (
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

var (
	ErrSlashCatalogUnsupported = fmt.Errorf("agent: provider does not support deterministic slash catalog discovery: %w", apperr.ErrUnprocessable)
	ErrSlashCatalogNoLiveTUI   = fmt.Errorf("agent: chat has no live provider TUI for slash catalog discovery: %w", apperr.ErrUnprocessable)
	ErrSlashCatalogTimeout     = fmt.Errorf("agent: slash catalog discovery timed out: %w", apperr.ErrTimeout)
	ErrSlashCatalogUnavailable = fmt.Errorf("agent: provider command for slash catalog discovery is unavailable: %w", engineterminal.ErrCommandNotFound)
	ErrSlashCatalogOutputLimit = fmt.Errorf("agent: slash catalog command exceeded its safe output limit: %w", apperr.ErrBadGateway)
	ErrSlashCatalogCommand     = fmt.Errorf("agent: slash catalog provider command failed: %w", apperr.ErrBadGateway)
	ErrSlashCatalogMalformed   = fmt.Errorf("agent: slash catalog provider output was malformed: %w", apperr.ErrBadGateway)
	ErrSlashCatalogSuperseded  = fmt.Errorf("agent: slash catalog request was superseded by a newer request: %w", apperr.ErrConflict)
)

// ErrProviderExitedDuringStartup is returned when a provider's vendor CLI died
// before its runner row could even be persisted.
//
// Crowbar hosts the ordinary INTERACTIVE CLI in a real PTY — the engine's hardest
// constraint, asserted by every descriptor's spawn.interactive_required — so a
// process that exits on the spot has not started, whatever its exit code. Handing
// the caller a chat with a corpse behind it would be a chat whose pane attaches to
// a PTY that is already gone.
//
// It wraps apperr.ErrFailedDependency (424), the same class as a vendor CLI that
// is not installed at all: the request was well-formed and the daemon is healthy,
// and what failed is a dependency the user can act on — an expired login, a broken
// install, a CLI that refuses this workspace.
var ErrProviderExitedDuringStartup = fmt.Errorf(
	"agent: spawn runner: provider process exited during startup: %w", apperr.ErrFailedDependency)

// ErrProviderDisabled is returned when a request names an agent provider the
// user has switched OFF in the global preference table.
//
// Disabled is a decision the preference table records and ResolveProviders
// reports, but reporting is not enforcing: a spawn or a provider switch names a
// provider id directly and never passes through the list that Enabled flag
// decorates, so a stale tab, a second window, or the command line would
// otherwise launch a provider the user turned off.
//
// It wraps apperr.ErrInvalidArgument, so handlers answer 400 through the
// existing sentinel mapping with no new case.
var ErrProviderDisabled = fmt.Errorf("agent: provider disabled: %w", apperr.ErrInvalidArgument)

var (
	// ErrPromptBusy means the chat began working, or still has a replacement TUI
	// awaiting its user_prompt hook, before this request acquired the spawn gate.
	ErrPromptBusy = fmt.Errorf("agent: chat is busy: %w", apperr.ErrConflict)

	// ErrPromptRequestIDConflict means one clientRequestId was reused for different
	// prompt text. Reusing an id can never silently submit a different operation.
	ErrPromptRequestIDConflict = fmt.Errorf("agent: client request id was already used for a different prompt: %w", apperr.ErrConflict)

	// ErrPromptOutcomeUnknown is the durable at-most-once recovery answer: Crowbar
	// wrote the dispatch intent but crashed before it could confirm the replacement
	// runner. Retrying automatically could duplicate a prompt the provider accepted.
	ErrPromptOutcomeUnknown = fmt.Errorf("agent: prior prompt delivery outcome is uncertain; inspect the chat before retrying with a new request id: %w", apperr.ErrConflict)

	// ErrPromptAlreadyAccepted is crash recovery with positive hook evidence: the
	// provider accepted the text, but Crowbar did not durably record the replacement
	// PTY identity before restarting. The client reconciles from the ledger and must
	// not submit it again.
	ErrPromptAlreadyAccepted = fmt.Errorf("agent: prompt was already accepted by the provider; reconcile from chat messages: %w", apperr.ErrConflict)

	// ErrPromptUnsupported leaves the native terminal fully usable while telling
	// React that this provider has no declarative restart-TUI submission mapping.
	ErrPromptUnsupported = fmt.Errorf("agent: provider does not support React prompt submission: %w", apperr.ErrUnprocessable)

	// ErrPromptSessionUnavailable means there is no live native TUI to replace.
	// A live lazy TUI need not have announced a session yet; that is a safe fresh
	// submission, not this error.
	ErrPromptSessionUnavailable = fmt.Errorf("agent: chat has no live provider TUI for prompt submission: %w", apperr.ErrUnprocessable)
)

const (
	PromptCodeBusy              = "chat_busy"
	PromptCodeOutcomeUnknown    = "request_outcome_uncertain"
	PromptCodeAlreadyAccepted   = "request_already_accepted"
	PromptCodeRequestIDConflict = "request_id_conflict"
	PromptCodeUnsupported       = "prompt_submit_unsupported"
	PromptCodeSessionRequired   = "live_tui_required"
)

const (
	CatalogCodeUnsupported  = "catalog_unsupported"
	CatalogCodeLiveRequired = "catalog_live_tui_required"
	CatalogCodeTimeout      = "catalog_timeout"
	CatalogCodeUnavailable  = "catalog_command_unavailable"
	CatalogCodeOutputLimit  = "catalog_output_limit"
	CatalogCodeCommand      = "catalog_command_failed"
	CatalogCodeMalformed    = "catalog_malformed_output"
	CatalogCodeSuperseded   = "catalog_superseded"
)

// PromptErrorCode returns the stable machine-readable API code for a prompt
// submission error. The human message may evolve; clients branch only on this.
func PromptErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrPromptBusy):
		return PromptCodeBusy
	case errors.Is(err, ErrPromptOutcomeUnknown):
		return PromptCodeOutcomeUnknown
	case errors.Is(err, ErrPromptAlreadyAccepted):
		return PromptCodeAlreadyAccepted
	case errors.Is(err, ErrPromptRequestIDConflict):
		return PromptCodeRequestIDConflict
	case errors.Is(err, ErrPromptUnsupported):
		return PromptCodeUnsupported
	case errors.Is(err, ErrPromptSessionUnavailable):
		return PromptCodeSessionRequired
	default:
		return ""
	}
}

func CatalogErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrSlashCatalogUnsupported):
		return CatalogCodeUnsupported
	case errors.Is(err, ErrSlashCatalogNoLiveTUI):
		return CatalogCodeLiveRequired
	case errors.Is(err, ErrSlashCatalogTimeout):
		return CatalogCodeTimeout
	case errors.Is(err, ErrSlashCatalogUnavailable):
		return CatalogCodeUnavailable
	case errors.Is(err, ErrSlashCatalogOutputLimit):
		return CatalogCodeOutputLimit
	case errors.Is(err, ErrSlashCatalogCommand):
		return CatalogCodeCommand
	case errors.Is(err, ErrSlashCatalogMalformed):
		return CatalogCodeMalformed
	case errors.Is(err, ErrSlashCatalogSuperseded):
		return CatalogCodeSuperseded
	default:
		return ""
	}
}
