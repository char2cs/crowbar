package runner

import (
	"errors"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/adapter/store/agentjournal"
	"github.com/char2cs/crowbar/api/internal/app/apperr"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
)

var (
	ErrSlashCatalogUnsupported = fmt.Errorf("agent: provider does not support deterministic slash catalog discovery: %w", apperr.ErrUnprocessable)
	ErrSlashCatalogTimeout     = fmt.Errorf("agent: slash catalog discovery timed out: %w", apperr.ErrTimeout)
	ErrSlashCatalogUnavailable = fmt.Errorf("agent: provider command for slash catalog discovery is unavailable: %w", engineterminal.ErrCommandNotFound)
	ErrSlashCatalogOutputLimit = fmt.Errorf("agent: slash catalog command exceeded its safe output limit: %w", apperr.ErrBadGateway)
	ErrSlashCatalogCommand     = fmt.Errorf("agent: slash catalog provider command failed: %w", apperr.ErrBadGateway)
	ErrSlashCatalogMalformed   = fmt.Errorf("agent: slash catalog provider output was malformed: %w", apperr.ErrBadGateway)
	ErrSlashCatalogSuperseded  = fmt.Errorf("agent: slash catalog request was superseded by a newer request: %w", apperr.ErrConflict)
)

// ErrStopped is returned by a switch or resume that was parked (on the
// outgoing turn, or on a prompt delivery) when the user pressed Stop. Stop
// preempts the parked request rather than queueing behind it (invariant A2),
// and the preempted request has changed nothing.
var ErrStopped = fmt.Errorf("agent: the chat was stopped while this request waited: %w", apperr.ErrConflict)

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
	"agent: spawn runner: provider process exited during startup: %w", apperr.ErrFailedDependency,
)

// ErrChatProviderUnknown is returned when a dormant chat cannot be resumed
// because nothing left on disk says which provider ever ran in it: no
// conversation history, no provider-switch marker, no placement history and no
// durable choice on the chat itself.
//
// It exists because this path used to answer agentrunner.ErrNotFound, which was
// a lie about which thing was missing. A dormant chat having no live runner is
// the NORMAL state and the entire reason Resume is being called — reporting
// "agentrunner not found" sent every reader looking for an absent runner, when
// the absent thing is the PROVIDER. Nothing is ever resolved by finding a
// runner here, so nothing should ever say one was looked for.
//
// It wraps apperr.ErrNotFound, so it keeps the 404 the old sentinel mapped to:
// the request is well-formed and the chat exists, but the record being resolved
// — the provider this chat ran — genuinely is not there. The client's correct
// response is to ASK which provider to start, never to pick one, which is what
// the previous indistinguishable error let it do.
var ErrChatProviderUnknown = fmt.Errorf(
	"agent: this chat no longer records which provider it ran: %w", apperr.ErrNotFound,
)

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

	// ErrSpawnPromptUndeliverable means a spawn was handed a user message and
	// could not get it to any carrier. A spawn carries a prompt in exactly one
	// of two places — the forked PTY's own argv, or a Dispatch down the api
	// connection it adopted INSTEAD of forking one — and a PTY-less spawn has
	// only the second. Failing here is the point: reporting success with the
	// message nowhere is what silently lost a user's prompt (see
	// carryPromptOverAPIConn), and the caller turns this into the journal's
	// "uncertain", which keeps the text in the client's own queue.
	ErrSpawnPromptUndeliverable = fmt.Errorf("agent: spawn runner: the prompt this spawn carries reached no carrier: %w", apperr.ErrBadGateway)

	// ErrPromptSessionUnavailable means there is no live native TUI to replace.
	// A live lazy TUI need not have announced a session yet; that is a safe fresh
	// submission, not this error.
	ErrPromptSessionUnavailable = fmt.Errorf("agent: chat has no live provider TUI for prompt submission: %w", apperr.ErrUnprocessable)
)

// promptJournalError translates the prompt journal's own refusals into this
// package's sentinels, at the one boundary where the store's answer becomes the
// caller's. The store cannot declare these itself: they wrap apperr classes,
// which live above it — and their IDENTITY is what handlers switch on for an
// HTTP status, so the translation must produce the very same values.
func promptJournalError(err error) error {
	switch {
	case errors.Is(err, agentjournal.ErrPromptBusy):
		return ErrPromptBusy
	case errors.Is(err, agentjournal.ErrPromptRequestIDConflict):
		return ErrPromptRequestIDConflict
	case errors.Is(err, agentjournal.ErrPromptOutcomeUnknown):
		return ErrPromptOutcomeUnknown
	default:
		return err
	}
}
