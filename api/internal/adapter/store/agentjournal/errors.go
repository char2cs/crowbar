package agentjournal

import "errors"

// The journals answer with their OWN plain sentinels. Translating them into
// app-level errors — the ones that carry an apperr class and therefore an HTTP
// status — is the caller's responsibility: a store that reached up for those
// would invert the layering.
var (
	// ErrPromptBusy means another request is already dispatching or spawned in
	// this journal, so a second delivery cannot be started.
	ErrPromptBusy = errors.New("agentjournal: prompt request: chat is busy")

	// ErrPromptRequestIDConflict means one request id was reused for different
	// prompt text. Reusing an id can never silently submit a different operation.
	ErrPromptRequestIDConflict = errors.New("agentjournal: prompt request: id was already used for a different prompt")

	// ErrPromptOutcomeUnknown means the journal holds a dispatch intent whose
	// outcome was never recorded: Crowbar crashed between writing the intent and
	// confirming the replacement runner.
	ErrPromptOutcomeUnknown = errors.New("agentjournal: prompt request: prior delivery outcome is uncertain")

	// ErrHookPayloadMismatch means one hook delivery id arrived twice carrying
	// different payloads, so deduplicating it would drop a distinct event.
	ErrHookPayloadMismatch = errors.New("agentjournal: hook delivery id reused with different payload")
)
