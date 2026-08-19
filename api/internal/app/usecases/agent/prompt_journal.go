package agent

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/char2cs/crowbar/api/internal/api/v0/dto"
)

const (
	promptRequestDirName = "prompt-requests"
	promptJournalMax     = 4096
	promptJournalMaxAge  = 30 * 24 * time.Hour

	promptStateDispatching = "dispatching"
	promptStateSpawned     = "spawned"
	promptStateAccepted    = "accepted"
	promptStateFailed      = "failed"
	promptStateUncertain   = "uncertain"
)

// promptRequestRecord is Crowbar's durable at-most-once delivery journal. It
// deliberately stores only a digest of the prompt; the hook-derived ledger is
// the sole durable conversation transcript.
type promptRequestRecord struct {
	RequestID         string    `json:"requestId"`
	TextHash          string    `json:"textHash"`
	State             string    `json:"state"`
	ProviderID        string    `json:"providerId"`
	OutgoingRunnerID  string    `json:"outgoingRunnerId,omitempty"`
	RunnerID          string    `json:"runnerId,omitempty"`
	TerminalSessionID string    `json:"terminalSessionId,omitempty"`
	CreatedAt         time.Time `json:"createdAt"`
	UpdatedAt         time.Time `json:"updatedAt"`
}

func (r promptRequestRecord) result() dto.PromptSubmissionDTO {
	return dto.PromptSubmissionDTO{
		RunnerID:          r.RunnerID,
		TerminalSessionID: r.TerminalSessionID,
	}
}

// promptJournal serializes request-record transitions against hook confirmation.
// The chat spawn gate serializes submissions, but hooks intentionally never take
// that gate and may confirm argv delivery the instant a replacement CLI starts.
type promptJournal struct {
	mu      sync.Mutex
	syncDir func(string) error
}

func newPromptJournal() *promptJournal {
	return &promptJournal{syncDir: syncPromptJournalDir}
}

func (j *promptJournal) write(dir string, record promptRequestRecord) error {
	return writePromptRecord(dir, record, j.syncDir)
}

func promptTextHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

func promptJournalDir(chatDir string) string {
	return filepath.Join(chatDir, promptRequestDirName)
}

func (j *promptJournal) lookup(
	dir, requestID, textHash string,
) (promptRequestRecord, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	record, found, err := readPromptRecord(dir, requestID)
	if err != nil || !found {
		return record, found, err
	}
	if record.TextHash != textHash {
		return promptRequestRecord{}, false, ErrPromptRequestIDConflict
	}
	// SubmitPrompt serializes on the chat gate. If a later request observes a
	// dispatching record, the attempt that wrote it is no longer in its
	// synchronous spawn section; it crashed or returned uncertain before it
	// could commit a result. Persist that truth lazily so it cannot block all
	// future request ids until the next daemon restart.
	if record.State == promptStateDispatching {
		record.State = promptStateUncertain
		record.UpdatedAt = time.Now().UTC()
		if err := j.write(dir, record); err != nil {
			return promptRequestRecord{}, false, err
		}
	}
	return record, true, nil
}

// begin returns an existing same-request record, or persists the dispatching
// intent before the caller performs any destructive process mutation. A
// different active request blocks the no-hook window between replacement spawn
// and the provider accepting its argv prompt.
func (j *promptJournal) begin(
	dir, requestID, textHash, providerID, outgoingRunnerID, replacementRunnerID string,
	now time.Time,
) (promptRequestRecord, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	_, statErr := os.Stat(dir)
	dirWasMissing := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !dirWasMissing {
		return promptRequestRecord{}, false, fmt.Errorf("agent: prompt journal: stat dir: %w", statErr)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return promptRequestRecord{}, false, fmt.Errorf("agent: prompt journal: mkdir: %w", err)
	}
	if dirWasMissing {
		// MkdirAll returning only says the directory is visible to this process.
		// Persist the new prompt-requests entry in the chat directory before the
		// first intent is allowed to precede destructive process mutation.
		if err := j.syncDir(filepath.Dir(dir)); err != nil {
			return promptRequestRecord{}, false, err
		}
	}
	existing, found, err := readPromptRecord(dir, requestID)
	if err != nil {
		return promptRequestRecord{}, false, err
	}
	if found {
		if existing.TextHash != textHash {
			return promptRequestRecord{}, false, ErrPromptRequestIDConflict
		}
		switch existing.State {
		case promptStateSpawned, promptStateAccepted, promptStateDispatching:
			return existing, true, nil
		case promptStateFailed:
			// Only failures proven to happen before the replacement process
			// starts reach this state. The same id may safely retry that text.
		case promptStateUncertain:
			return existing, true, nil
		default:
			return promptRequestRecord{}, false, fmt.Errorf("agent: prompt journal: invalid state %q", existing.State)
		}
	}

	if err := recoverOrphanedDispatchesLocked(dir, now, j.write); err != nil {
		return promptRequestRecord{}, false, err
	}
	active, err := activePromptRecord(dir, requestID)
	if err != nil {
		return promptRequestRecord{}, false, err
	}
	if active.State == promptStateDispatching {
		return promptRequestRecord{}, false, ErrPromptOutcomeUnknown
	}
	if active.State == promptStateSpawned {
		return promptRequestRecord{}, false, ErrPromptBusy
	}

	record := promptRequestRecord{
		RequestID:        requestID,
		TextHash:         textHash,
		State:            promptStateDispatching,
		ProviderID:       providerID,
		OutgoingRunnerID: outgoingRunnerID,
		RunnerID:         replacementRunnerID,
		CreatedAt:        now.UTC(),
		UpdatedAt:        now.UTC(),
	}
	if err := j.write(dir, record); err != nil {
		return promptRequestRecord{}, false, err
	}
	return record, false, nil
}

func (j *promptJournal) markSpawned(
	dir, requestID, textHash string,
	result dto.PromptSubmissionDTO,
	now time.Time,
) (promptRequestRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()

	record, found, err := readPromptRecord(dir, requestID)
	if err != nil {
		return promptRequestRecord{}, err
	}
	if !found || record.TextHash != textHash {
		return promptRequestRecord{}, fmt.Errorf("agent: prompt journal: dispatch record disappeared")
	}
	record.RunnerID = result.RunnerID
	record.TerminalSessionID = result.TerminalSessionID
	// A fast user_prompt hook or PTY exit may already have confirmed or made
	// this request uncertain while spawnRunner was returning. Only the original
	// dispatching state advances to spawned; never overwrite either outcome.
	if record.State == promptStateDispatching {
		record.State = promptStateSpawned
	}
	record.UpdatedAt = now.UTC()
	if err := j.write(dir, record); err != nil {
		return promptRequestRecord{}, err
	}
	prunePromptRecords(dir, now)
	return record, nil
}

// confirmAccepted correlates the provider's real user_prompt hook with the one
// active dispatch by runner, provider and text digest. It stores no plaintext.
func (j *promptJournal) confirmAccepted(
	dir, runnerID, providerID, textHash string,
	now time.Time,
) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	records, err := readPromptRecords(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, record := range records {
		if record.State != promptStateDispatching && record.State != promptStateSpawned && record.State != promptStateUncertain {
			continue
		}
		if record.TextHash != textHash || record.ProviderID != providerID {
			continue
		}
		if record.RunnerID == "" && record.OutgoingRunnerID == runnerID {
			continue
		}
		if record.RunnerID != "" && record.RunnerID != runnerID {
			continue
		}
		record.State = promptStateAccepted
		record.RunnerID = runnerID
		record.UpdatedAt = now.UTC()
		return j.write(dir, record)
	}
	return nil
}

func (j *promptJournal) markUncertain(
	dir, requestID string,
	now time.Time,
) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	record, found, err := readPromptRecord(dir, requestID)
	if err != nil || !found {
		return err
	}
	if record.State != promptStateSpawned && record.State != promptStateDispatching {
		return nil
	}
	record.State = promptStateUncertain
	record.UpdatedAt = now.UTC()
	return j.write(dir, record)
}

func (j *promptJournal) markFailedDispatch(
	dir, requestID string,
	now time.Time,
) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	record, found, err := readPromptRecord(dir, requestID)
	if err != nil || !found {
		return err
	}
	if record.State != promptStateDispatching {
		return nil
	}
	record.State = promptStateFailed
	record.UpdatedAt = now.UTC()
	return j.write(dir, record)
}

// repointDispatch re-attributes an in-flight dispatch to the runner that will
// actually deliver it.
//
// It exists for exactly one transition: a rewake handoff that found no collector,
// falling back to a restart. That dispatch was journaled against the LIVE runner,
// because a rewake is delivered by the process already running — and the restart
// is about to kill that process, at which point its departure reconciliation would
// find an active record pointing at it, see no matching turn, and correctly report
// an outcome nobody can determine. Correctly, but about a message that was
// provably never delivered.
//
// It only ever moves a record that is still DISPATCHING, so it can never disturb
// an outcome already decided, and it never touches state — only the attribution.
func (j *promptJournal) repointDispatch(
	dir, requestID, runnerID string,
	now time.Time,
) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	record, found, err := readPromptRecord(dir, requestID)
	if err != nil || !found {
		return err
	}
	if record.State != promptStateDispatching {
		return nil
	}
	record.RunnerID = runnerID
	record.UpdatedAt = now.UTC()
	return j.write(dir, record)
}

func (j *promptJournal) markAcceptedByRequest(
	dir, requestID string,
	now time.Time,
) (promptRequestRecord, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	record, found, err := readPromptRecord(dir, requestID)
	if err != nil {
		return promptRequestRecord{}, err
	}
	if !found {
		return promptRequestRecord{}, fmt.Errorf("agent: prompt journal: request not found")
	}
	record.State = promptStateAccepted
	record.UpdatedAt = now.UTC()
	return record, j.write(dir, record)
}

func (j *promptJournal) activeForRunner(
	dir, runnerID, providerID string,
) (promptRequestRecord, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	records, err := readPromptRecords(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return promptRequestRecord{}, false, nil
		}
		return promptRequestRecord{}, false, err
	}
	for _, record := range records {
		if record.State != promptStateSpawned && record.State != promptStateDispatching {
			continue
		}
		if record.ProviderID != providerID {
			continue
		}
		// A dispatching record has no replacement runner id yet. It must not
		// attach itself to the outgoing same-provider runner that SubmitPrompt is
		// currently displacing; only mark a record failed for the exact runner
		// durably written after spawn.
		if record.RunnerID != "" && record.RunnerID == runnerID {
			return record, true, nil
		}
	}
	return promptRequestRecord{}, false, nil
}

func (j *promptJournal) hasPendingDelivery(dir string) (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := recoverOrphanedDispatchesLocked(dir, time.Now(), j.write); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	record, err := activePromptRecord(dir, "")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return record.RequestID != "", nil
}

func (j *promptJournal) activeDelivery(dir string) (promptRequestRecord, bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	record, err := activePromptRecord(dir, "")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return promptRequestRecord{}, false, nil
		}
		return promptRequestRecord{}, false, err
	}
	return record, record.RequestID != "", nil
}

// recoverOrphanedDispatches runs after a daemon restart and lazily under the
// per-chat spawn gate. A dispatching record is an attempt whose synchronous
// submit section did not reach a durable result. No in-memory callback survived
// (or no prior submit still holds the gate) to resolve it, so it must become
// explicitly uncertain; leaving it dispatching would block the chat forever.
func (j *promptJournal) recoverOrphanedDispatches(dir string, now time.Time) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	return recoverOrphanedDispatchesLocked(dir, now, j.write)
}

func recoverOrphanedDispatchesLocked(
	dir string,
	now time.Time,
	write func(string, promptRequestRecord) error,
) error {
	records, err := readPromptRecords(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, record := range records {
		if record.State != promptStateDispatching {
			continue
		}
		record.State = promptStateUncertain
		record.UpdatedAt = now.UTC()
		if err := write(dir, record); err != nil {
			return err
		}
	}
	return nil
}

func activePromptRecord(dir, exceptID string) (promptRequestRecord, error) {
	records, err := readPromptRecords(dir)
	if err != nil {
		return promptRequestRecord{}, err
	}
	for _, record := range records {
		if record.RequestID == exceptID {
			continue
		}
		if record.State == promptStateDispatching || record.State == promptStateSpawned {
			return record, nil
		}
	}
	return promptRequestRecord{}, nil
}

func readPromptRecords(dir string) ([]promptRequestRecord, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("agent: prompt journal: readdir: %w", err)
	}
	records := make([]promptRequestRecord, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, found, err := readPromptRecordPath(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if found {
			records = append(records, record)
		}
	}
	return records, nil
}

func readPromptRecord(dir, requestID string) (promptRequestRecord, bool, error) {
	return readPromptRecordPath(filepath.Join(dir, requestID+".json"))
}

func readPromptRecordPath(path string) (promptRequestRecord, bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is daemon-generated under the chat journal
	if errors.Is(err, os.ErrNotExist) {
		return promptRequestRecord{}, false, nil
	}
	if err != nil {
		return promptRequestRecord{}, false, fmt.Errorf("agent: prompt journal: read: %w", err)
	}
	var record promptRequestRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return promptRequestRecord{}, false, fmt.Errorf("agent: prompt journal: decode: %w", err)
	}
	return record, true, nil
}

func writePromptRecord(
	dir string,
	record promptRequestRecord,
	syncDir func(string) error,
) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("agent: prompt journal: encode: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".prompt-request-*")
	if err != nil {
		return fmt.Errorf("agent: prompt journal: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: prompt journal: chmod temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: prompt journal: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: prompt journal: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("agent: prompt journal: close temp: %w", err)
	}
	target := filepath.Join(dir, record.RequestID+".json")
	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("agent: prompt journal: commit: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	return nil
}

// syncPromptJournalDir makes the rename durable. A successful rename with an
// unsynced parent is not a durable intent: power loss may erase the directory
// entry even though the process has already moved on to destructive TUI teardown.
// Every open/sync/close error is therefore part of the journal write result.
func syncPromptJournalDir(dir string) error {
	dh, err := os.Open(dir) //nolint:gosec // daemon-owned journal directory
	if err != nil {
		return fmt.Errorf("agent: prompt journal: open parent for sync: %w", err)
	}
	syncErr := dh.Sync()
	closeErr := dh.Close()
	if syncErr != nil || closeErr != nil {
		return fmt.Errorf("agent: prompt journal: sync parent: %w", errors.Join(syncErr, closeErr))
	}
	return nil
}

// prunePromptRecords bounds durable idempotency history without ever removing
// an active request. It is best-effort: failure to clean old metadata must not
// turn a successful provider spawn into an HTTP failure.
func prunePromptRecords(dir string, now time.Time) {
	records, err := readPromptRecords(dir)
	if err != nil {
		return
	}
	sort.Slice(records, func(i, k int) bool { return records[i].UpdatedAt.Before(records[k].UpdatedAt) })
	removable := make([]promptRequestRecord, 0, len(records))
	for _, record := range records {
		// Uncertain is an unresolved at-most-once tombstone, not ordinary
		// history. A frontend may retry its UUID indefinitely; deleting it would
		// turn that retry into a fresh duplicate. It is therefore retained without
		// age/count expiry until positive acceptance evidence changes its state.
		if record.State != promptStateDispatching &&
			record.State != promptStateSpawned &&
			record.State != promptStateUncertain {
			removable = append(removable, record)
		}
	}
	remaining := len(records)
	for _, record := range removable {
		if now.Sub(record.UpdatedAt) <= promptJournalMaxAge && remaining <= promptJournalMax {
			continue
		}
		if os.Remove(filepath.Join(dir, record.RequestID+".json")) == nil {
			remaining--
		}
	}
}
