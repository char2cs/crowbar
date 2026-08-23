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

	promptStateSettled = "settled"
)

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

	if record.State == promptStateDispatching {
		record.State = promptStateUncertain
		record.UpdatedAt = time.Now().UTC()
		if err := j.write(dir, record); err != nil {
			return promptRequestRecord{}, false, err
		}
	}
	return record, true, nil
}

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
		case promptStateSpawned, promptStateAccepted, promptStateSettled, promptStateDispatching:
			return existing, true, nil
		case promptStateFailed:

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
		if record.State != promptStateDispatching &&
			record.State != promptStateSpawned &&
			record.State != promptStateUncertain &&
			record.State != promptStateSettled {
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

func (j *promptJournal) settle(
	dir, requestID string,
	now time.Time,
) (bool, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	record, found, err := readPromptRecord(dir, requestID)
	if err != nil || !found {
		return false, err
	}
	if record.State != promptStateSpawned {
		return false, nil
	}
	record.State = promptStateSettled
	record.UpdatedAt = now.UTC()
	if err := j.write(dir, record); err != nil {
		return false, err
	}
	return true, nil
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

func prunePromptRecords(dir string, now time.Time) {
	records, err := readPromptRecords(dir)
	if err != nil {
		return
	}
	sort.Slice(records, func(i, k int) bool { return records[i].UpdatedAt.Before(records[k].UpdatedAt) })
	removable := make([]promptRequestRecord, 0, len(records))
	for _, record := range records {
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
