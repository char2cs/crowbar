package agentjournal

import (
	"os"
	"time"
)

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

// WriteRecord runs the generic atomic-write sequence directly, so a test can
// aim a fault at each of its steps (encode, temp-file creation, the durability
// commit inside stageRecord, or the final rename) without going through one of
// the two concrete journals built on top of it.
func WriteRecord(
	dir string,
	name string,
	record any,
	tempPattern string,
	syncDir func(string) error,
) error {
	return writeRecord(dir, name, record, tempPattern, syncDir)
}

// ReadRecord runs the generic decode straight off disk, so a test can plant a
// corrupt or unreadable record and confirm the failure is reported rather than
// silently treated as "not found".
func ReadRecord(
	path string,
	into any,
) (bool, error) {
	return readRecord(path, into)
}

// StageRecord runs the write-to-an-already-open-temp-file step directly, so a
// test can hand it a file descriptor in a state production never produces
// (already closed, opened read-only) to reach the chmod/write/sync/close
// failure branches that writeRecord's own os.CreateTemp can't be made to fail
// into.
func StageRecord(
	tmp *os.File,
	data []byte,
) error {
	return stageRecord(tmp, data)
}

// SyncJournalDir runs the parent-directory fsync directly, so a test can point
// it at a directory writeRecord's own callers never would (one that has since
// vanished) to reach the os.Open failure branch.
func SyncJournalDir(
	dir string,
) error {
	return syncJournalDir(dir)
}
