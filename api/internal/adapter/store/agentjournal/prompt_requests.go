package agentjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// The states a prompt request moves through. They are on disk, so they are the
// vocabulary a caller crash-recovers with, not an internal detail.
const (
	// PromptStateDispatching is the intent written BEFORE the outgoing TUI is
	// touched. Finding one after a restart means the outcome is unknown.
	PromptStateDispatching = "dispatching"
	// PromptStateSpawned means the replacement process exists and its identity
	// was durably recorded.
	PromptStateSpawned = "spawned"
	// PromptStateAccepted means the provider itself acknowledged the text.
	PromptStateAccepted = "accepted"
	// PromptStateFailed is a PROVEN pre-spawn failure: the only state a retry of
	// the same request id may fall through.
	PromptStateFailed = "failed"
	// PromptStateUncertain means the delivery may or may not have reached the
	// provider, so it must never be retried automatically.
	PromptStateUncertain = "uncertain"
	// PromptStateSettled means a spawned delivery was retired without producing a
	// turn, so it no longer blocks the chat.
	PromptStateSettled = "settled"
)

const (
	promptRequestsDirName = "prompt-requests"
	promptRequestsMax     = 4096
	promptRequestsMaxAge  = 30 * 24 * time.Hour
	promptRequestTempPat  = ".prompt-request-*"
)

// PromptRequest is one durable prompt submission record. Its JSON field names
// are the on-disk format; a rename orphans every record already written.
type PromptRequest struct {
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

// PromptRequests is the durable at-most-once journal behind React prompt
// submission: one record per client request id, under a chat's own directory.
//
// Its transition methods also serialise against each other, because the same
// journal is written from the submission path and from the user_prompt hook
// that confirms it.
type PromptRequests interface {
	// Dir returns the journal directory for one chat.
	Dir(
		chatsDir string,
		chatID string,
	) string
	// Begin records a dispatching intent for requestID, creating the journal
	// directory if needed. It reports whether an ATTEMPT for this id already
	// existed (in which case the caller must classify it rather than dispatch),
	// and refuses with ErrPromptBusy, ErrPromptOutcomeUnknown or
	// ErrPromptRequestIDConflict when the journal already owes an answer.
	Begin(
		dir string,
		requestID string,
		textHash string,
		providerID string,
		outgoingRunnerID string,
		replacementRunnerID string,
		now time.Time,
	) (PromptRequest, bool, error)
	// Lookup returns the record for requestID, downgrading a dispatching record
	// to uncertain first: reading one back means nobody is dispatching it now. It
	// returns ErrPromptRequestIDConflict when the id was used for other text.
	Lookup(
		dir string,
		requestID string,
		textHash string,
	) (PromptRequest, bool, error)
	// MarkSpawned records the replacement process identity against a dispatching
	// record and prunes the journal. The returned record still reads uncertain if
	// something else downgraded it meanwhile.
	MarkSpawned(
		dir string,
		requestID string,
		textHash string,
		runnerID string,
		terminalSessionID string,
		now time.Time,
	) (PromptRequest, error)
	// ConfirmAccepted marks the unsettled record matching this runner, provider
	// and text as accepted. A journal with nothing matching is not an error: the
	// provider may be echoing a prompt Crowbar never submitted.
	ConfirmAccepted(
		dir string,
		runnerID string,
		providerID string,
		textHash string,
		now time.Time,
	) error
	// MarkUncertain downgrades a dispatching or spawned record, so no automatic
	// retry can duplicate a prompt the provider may already hold.
	MarkUncertain(
		dir string,
		requestID string,
		now time.Time,
	) error
	// MarkFailedDispatch records a PROVEN pre-spawn failure, the one outcome a
	// retry of the same request id may safely fall through.
	MarkFailedDispatch(
		dir string,
		requestID string,
		now time.Time,
	) error
	// MarkAccepted accepts a record by request id. An unknown id is an error:
	// accepting a request nobody journalled is a caller bug, not a state.
	MarkAccepted(
		dir string,
		requestID string,
		now time.Time,
	) (PromptRequest, error)
	// Settle retires a SPAWNED record that produced no turn, and reports whether
	// it retired one. Any other state is left exactly as it is.
	Settle(
		dir string,
		requestID string,
		now time.Time,
	) (bool, error)
	// ActiveForRunner returns the in-flight record a given runner is delivering.
	ActiveForRunner(
		dir string,
		runnerID string,
		providerID string,
	) (PromptRequest, bool, error)
	// ActiveDelivery returns the journal's single in-flight record, if any.
	ActiveDelivery(
		dir string,
	) (PromptRequest, bool, error)
	// HasPendingDelivery reports whether anything is genuinely in flight, first
	// downgrading orphaned dispatches: an unknown outcome blocks a RETRY, but it
	// is not a delivery still on its way.
	HasPendingDelivery(
		dir string,
	) (bool, error)
	// RecoverOrphanedDispatches downgrades every dispatching record to uncertain.
	// It is what boot runs: a dispatching record can only be written by a process
	// that is no longer running.
	RecoverOrphanedDispatches(
		dir string,
		now time.Time,
	) error
}

// PromptTextHash is the digest a request id is bound to, so one id can never
// silently submit different text.
func PromptTextHash(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}

// NewPromptRequests builds the prompt request journal.
func NewPromptRequests(
	opts ...Option,
) PromptRequests {
	return newPromptRequests(opts...)
}

type promptRequests struct {
	mu      sync.Mutex
	syncDir func(string) error
}

func newPromptRequests(
	opts ...Option,
) *promptRequests {
	return &promptRequests{syncDir: resolve(opts).syncDir}
}

func (s *promptRequests) Dir(
	chatsDir string,
	chatID string,
) string {
	return filepath.Join(chatsDir, chatID, promptRequestsDirName)
}

func (s *promptRequests) write(dir string, record PromptRequest) error {
	return writePromptRequest(dir, record, s.syncDir)
}

func (s *promptRequests) Lookup(
	dir string,
	requestID string,
	textHash string,
) (PromptRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, found, err := readPromptRequest(dir, requestID)
	if err != nil || !found {
		return record, found, err
	}
	if record.TextHash != textHash {
		return PromptRequest{}, false, ErrPromptRequestIDConflict
	}

	if record.State == PromptStateDispatching {
		record.State = PromptStateUncertain
		record.UpdatedAt = time.Now().UTC()
		if err := s.write(dir, record); err != nil {
			return PromptRequest{}, false, err
		}
	}
	return record, true, nil
}

func (s *promptRequests) Begin(
	dir string,
	requestID string,
	textHash string,
	providerID string,
	outgoingRunnerID string,
	replacementRunnerID string,
	now time.Time,
) (PromptRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.ensureDir(dir); err != nil {
		return PromptRequest{}, false, err
	}
	existing, found, err := readPromptRequest(dir, requestID)
	if err != nil {
		return PromptRequest{}, false, err
	}
	if found {
		reused, done, reuseErr := reusablePromptRequest(existing, textHash)
		if done || reuseErr != nil {
			return reused, done, reuseErr
		}
	}

	if err := recoverOrphanedDispatchesLocked(dir, now, s.write); err != nil {
		return PromptRequest{}, false, err
	}
	if err := requireNoActiveRequest(dir, requestID); err != nil {
		return PromptRequest{}, false, err
	}
	record := PromptRequest{
		RequestID:        requestID,
		TextHash:         textHash,
		State:            PromptStateDispatching,
		ProviderID:       providerID,
		OutgoingRunnerID: outgoingRunnerID,
		RunnerID:         replacementRunnerID,
		CreatedAt:        now.UTC(),
		UpdatedAt:        now.UTC(),
	}
	if err := s.write(dir, record); err != nil {
		return PromptRequest{}, false, err
	}
	return record, false, nil
}

func (s *promptRequests) ensureDir(dir string) error {
	_, statErr := os.Stat(dir)
	missing := errors.Is(statErr, os.ErrNotExist)
	if statErr != nil && !missing {
		return fmt.Errorf("agentjournal: prompt request: stat dir: %w", statErr)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("agentjournal: prompt request: mkdir: %w", err)
	}
	if !missing {
		return nil
	}
	return s.syncDir(filepath.Dir(dir))
}

func reusablePromptRequest(
	existing PromptRequest,
	textHash string,
) (PromptRequest, bool, error) {
	if existing.TextHash != textHash {
		return PromptRequest{}, false, ErrPromptRequestIDConflict
	}
	switch existing.State {
	case PromptStateSpawned, PromptStateAccepted, PromptStateSettled,
		PromptStateDispatching, PromptStateUncertain:
		return existing, true, nil
	case PromptStateFailed:
		return PromptRequest{}, false, nil
	default:
		return PromptRequest{}, false, fmt.Errorf("agentjournal: prompt request: invalid state %q", existing.State)
	}
}

func requireNoActiveRequest(dir, exceptID string) error {
	active, err := activePromptRequest(dir, exceptID)
	if err != nil {
		return err
	}
	if active.State == PromptStateDispatching {
		return ErrPromptOutcomeUnknown
	}
	if active.State == PromptStateSpawned {
		return ErrPromptBusy
	}
	return nil
}

func (s *promptRequests) MarkSpawned(
	dir string,
	requestID string,
	textHash string,
	runnerID string,
	terminalSessionID string,
	now time.Time,
) (PromptRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, found, err := readPromptRequest(dir, requestID)
	if err != nil {
		return PromptRequest{}, err
	}
	if !found || record.TextHash != textHash {
		return PromptRequest{}, fmt.Errorf("agentjournal: prompt request: dispatch record disappeared")
	}
	record.RunnerID = runnerID
	record.TerminalSessionID = terminalSessionID

	if record.State == PromptStateDispatching {
		record.State = PromptStateSpawned
	}
	record.UpdatedAt = now.UTC()
	if err := s.write(dir, record); err != nil {
		return PromptRequest{}, err
	}
	prunePromptRequests(dir, now)
	return record, nil
}

func (s *promptRequests) ConfirmAccepted(
	dir string,
	runnerID string,
	providerID string,
	textHash string,
	now time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	records, err := readPromptRequests(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, record := range records {
		if !acknowledgeable(record, runnerID, providerID, textHash) {
			continue
		}
		record.State = PromptStateAccepted
		record.RunnerID = runnerID
		record.UpdatedAt = now.UTC()
		return s.write(dir, record)
	}
	return nil
}

func acknowledgeable(
	record PromptRequest,
	runnerID string,
	providerID string,
	textHash string,
) bool {
	if record.State != PromptStateDispatching &&
		record.State != PromptStateSpawned &&
		record.State != PromptStateUncertain &&
		record.State != PromptStateSettled {
		return false
	}
	if record.TextHash != textHash || record.ProviderID != providerID {
		return false
	}
	if record.RunnerID == "" && record.OutgoingRunnerID == runnerID {
		return false
	}
	return record.RunnerID == "" || record.RunnerID == runnerID
}

func (s *promptRequests) MarkUncertain(
	dir string,
	requestID string,
	now time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, found, err := readPromptRequest(dir, requestID)
	if err != nil || !found {
		return err
	}
	if record.State != PromptStateSpawned && record.State != PromptStateDispatching {
		return nil
	}
	record.State = PromptStateUncertain
	record.UpdatedAt = now.UTC()
	return s.write(dir, record)
}

func (s *promptRequests) MarkFailedDispatch(
	dir string,
	requestID string,
	now time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, found, err := readPromptRequest(dir, requestID)
	if err != nil || !found {
		return err
	}
	if record.State != PromptStateDispatching {
		return nil
	}
	record.State = PromptStateFailed
	record.UpdatedAt = now.UTC()
	return s.write(dir, record)
}

func (s *promptRequests) MarkAccepted(
	dir string,
	requestID string,
	now time.Time,
) (PromptRequest, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, found, err := readPromptRequest(dir, requestID)
	if err != nil {
		return PromptRequest{}, err
	}
	if !found {
		return PromptRequest{}, fmt.Errorf("agentjournal: prompt request: request not found")
	}
	record.State = PromptStateAccepted
	record.UpdatedAt = now.UTC()
	return record, s.write(dir, record)
}

func (s *promptRequests) Settle(
	dir string,
	requestID string,
	now time.Time,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, found, err := readPromptRequest(dir, requestID)
	if err != nil || !found {
		return false, err
	}
	if record.State != PromptStateSpawned {
		return false, nil
	}
	record.State = PromptStateSettled
	record.UpdatedAt = now.UTC()
	if err := s.write(dir, record); err != nil {
		return false, err
	}
	return true, nil
}

func (s *promptRequests) ActiveForRunner(
	dir string,
	runnerID string,
	providerID string,
) (PromptRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	records, err := readPromptRequests(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PromptRequest{}, false, nil
		}
		return PromptRequest{}, false, err
	}
	for _, record := range records {
		if record.State != PromptStateSpawned && record.State != PromptStateDispatching {
			continue
		}
		if record.ProviderID != providerID {
			continue
		}

		if record.RunnerID != "" && record.RunnerID == runnerID {
			return record, true, nil
		}
	}
	return PromptRequest{}, false, nil
}

func (s *promptRequests) HasPendingDelivery(dir string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := recoverOrphanedDispatchesLocked(dir, time.Now(), s.write); err != nil && !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	record, err := activePromptRequest(dir, "")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return record.RequestID != "", nil
}

func (s *promptRequests) ActiveDelivery(dir string) (PromptRequest, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	record, err := activePromptRequest(dir, "")
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return PromptRequest{}, false, nil
		}
		return PromptRequest{}, false, err
	}
	return record, record.RequestID != "", nil
}

func (s *promptRequests) RecoverOrphanedDispatches(dir string, now time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return recoverOrphanedDispatchesLocked(dir, now, s.write)
}

func recoverOrphanedDispatchesLocked(
	dir string,
	now time.Time,
	write func(string, PromptRequest) error,
) error {
	records, err := readPromptRequests(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, record := range records {
		if record.State != PromptStateDispatching {
			continue
		}
		record.State = PromptStateUncertain
		record.UpdatedAt = now.UTC()
		if err := write(dir, record); err != nil {
			return err
		}
	}
	return nil
}

func activePromptRequest(dir, exceptID string) (PromptRequest, error) {
	records, err := readPromptRequests(dir)
	if err != nil {
		return PromptRequest{}, err
	}
	for _, record := range records {
		if record.RequestID == exceptID {
			continue
		}
		if record.State == PromptStateDispatching || record.State == PromptStateSpawned {
			return record, nil
		}
	}
	return PromptRequest{}, nil
}

func readPromptRequests(dir string) ([]PromptRequest, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("agentjournal: prompt request: readdir: %w", err)
	}
	records := make([]PromptRequest, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		record, found, err := readPromptRequestPath(filepath.Join(dir, entry.Name()))
		if err != nil {
			return nil, err
		}
		if found {
			records = append(records, record)
		}
	}
	return records, nil
}

func readPromptRequest(dir, requestID string) (PromptRequest, bool, error) {
	return readPromptRequestPath(filepath.Join(dir, requestID+".json"))
}

func readPromptRequestPath(path string) (PromptRequest, bool, error) {
	var record PromptRequest
	found, err := readRecord(path, &record)
	if err != nil || !found {
		return PromptRequest{}, false, err
	}
	return record, true, nil
}

func writePromptRequest(
	dir string,
	record PromptRequest,
	syncDir func(string) error,
) error {
	return writeRecord(dir, record.RequestID+".json", record, promptRequestTempPat, syncDir)
}

func prunePromptRequests(dir string, now time.Time) {
	records, err := readPromptRequests(dir)
	if err != nil {
		return
	}
	sort.Slice(records, func(i, k int) bool { return records[i].UpdatedAt.Before(records[k].UpdatedAt) })
	removable := make([]PromptRequest, 0, len(records))
	for _, record := range records {
		if record.State != PromptStateDispatching &&
			record.State != PromptStateSpawned &&
			record.State != PromptStateUncertain {
			removable = append(removable, record)
		}
	}
	remaining := len(records)
	for _, record := range removable {
		if now.Sub(record.UpdatedAt) <= promptRequestsMaxAge && remaining <= promptRequestsMax {
			continue
		}
		if os.Remove(filepath.Join(dir, record.RequestID+".json")) == nil {
			remaining--
		}
	}
}
