package agentjournal

import "time"

// WritePromptRequest commits one record with the production durability
// sequence. It is test-only surface: this file is compiled only under `go test`.
//
// It exists because a journal's interesting states are the ones NO caller can
// reach through the interface — a record left dispatching by a crash, a state
// no version of Crowbar ever wrote — and a test that cannot plant those can
// only assert the transitions that already work.
func WritePromptRequest(
	dir string,
	record PromptRequest,
) error {
	return writePromptRequest(dir, record, syncJournalDir)
}

// ReadPromptRequest reads one record straight off disk, bypassing the store's
// transitions. It is what proves a method was DURABLE before it returned, not
// merely correct in memory.
func ReadPromptRequest(
	dir string,
	requestID string,
) (PromptRequest, bool, error) {
	return readPromptRequest(dir, requestID)
}

// PrunePromptRequests runs the retention sweep directly. Production only
// reaches it amortised inside MarkSpawned, which cannot be aimed at a journal
// of long-settled records.
func PrunePromptRequests(
	dir string,
	now time.Time,
) {
	prunePromptRequests(dir, now)
}
