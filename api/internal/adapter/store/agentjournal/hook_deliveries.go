package agentjournal

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	// HookDeliveriesDirName is the per-workspace root the per-runner delivery
	// directories live under, beside the chats it is a sibling of.
	HookDeliveriesDirName = ".hook-deliveries"

	// HookDeliveryCompletedMax is the FIFO cap on the in-memory completion
	// markers. Past it a replay is answered from disk instead of from memory.
	HookDeliveryCompletedMax = 32

	// HookDeliveryJournalMax is the cap on COMPLETED records kept in one runner's
	// directory. An in-flight record is never pruned, whatever the count.
	HookDeliveryJournalMax = 128

	// HookDeliveryJournalMaxAge is the silence after which a whole runner
	// directory is reaped.
	HookDeliveryJournalMaxAge = 30 * 24 * time.Hour

	// HookDeliveryPruneEvery is how many completions apart the amortised on-disk
	// prune runs.
	HookDeliveryPruneEvery = 16

	hookDeliveryStatePending   = "pending"
	hookDeliveryStateCompleted = "completed"
	hookDeliveryScanMax        = 1024
	hookDeliveryTempPat        = ".hook-delivery-*"
)

// HookDelivery is one durable hook ingress record. Its JSON field names are the
// on-disk format; a rename orphans every record already written.
type HookDelivery struct {
	DeliveryID string    `json:"deliveryId"`
	Hash       string    `json:"hash"`
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

// HookDeliveries is the exactly-once ingress boundary for Crowbar's own hook
// relay: it durably remembers which delivery ids already ran their effects, so
// a retried POST mutates nothing twice. The relay owns retry and spooling; this
// journal owns only the question of whether an arrival is new.
//
// It does NOT serialise the work a delivery triggers. Ordering the ingestion of
// one runner's hooks is orchestration the caller holds across the whole ingest,
// far outside a store operation, so the caller keeps that lock.
type HookDeliveries interface {
	// Dir returns the journal directory for one runner's deliveries.
	Dir(
		chatsDir string,
		runnerID string,
	) string
	// Begin durably records deliveryID as in flight and reports whether it is
	// already DONE, in which case the caller must skip its effects entirely. It
	// returns ErrHookPayloadMismatch when the id arrives with a different payload.
	Begin(
		dir string,
		deliveryID string,
		hash string,
		now time.Time,
	) (bool, error)
	// Complete records that a delivery's effects committed, and amortises the
	// journal's retention sweep. A failure here leaves effects committed with no
	// marker, so the caller must log rather than retry.
	Complete(
		dir string,
		deliveryID string,
		hash string,
		now time.Time,
	) error
	// CompletionMarkers returns the delivery ids currently answerable from
	// memory. It is the discriminator for whether an answer came from the
	// in-memory markers or from the disk behind them.
	CompletionMarkers() []string
}

// HookDeliveryHash binds a delivery id to the exact payload it carried, so a
// reused id carrying different content is caught instead of deduplicated.
func HookDeliveryHash(
	runnerID string,
	provider string,
	event string,
	raw []byte,
) string {
	h := sha256.New()
	_, _ = h.Write([]byte(runnerID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(provider))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(event))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write(raw)
	return hex.EncodeToString(h.Sum(nil))
}

// NewHookDeliveries builds the hook delivery journal.
func NewHookDeliveries(
	opts ...Option,
) HookDeliveries {
	return newHookDeliveries(opts...)
}

type hookDeliveries struct {
	mu          sync.Mutex
	syncDir     func(string) error
	completed   map[string]string
	order       []string
	completions int
}

func newHookDeliveries(
	opts ...Option,
) *hookDeliveries {
	return &hookDeliveries{
		syncDir:   resolve(opts).syncDir,
		completed: map[string]string{},
	}
}

func (s *hookDeliveries) Dir(
	chatsDir string,
	runnerID string,
) string {
	return filepath.Join(chatsDir, HookDeliveriesDirName, runnerID)
}

func (s *hookDeliveries) CompletionMarkers() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.order...)
}

func (s *hookDeliveries) Begin(
	dir string,
	deliveryID string,
	hash string,
	now time.Time,
) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if completedHash, ok := s.completed[deliveryID]; ok {
		if completedHash != hash {
			return false, ErrHookPayloadMismatch
		}
		return true, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("agentjournal: hook delivery: mkdir: %w", err)
	}
	record, found, err := readHookDelivery(dir, deliveryID)
	if err != nil {
		return false, err
	}
	if found {
		if record.Hash != hash {
			return false, ErrHookPayloadMismatch
		}
		return record.State == hookDeliveryStateCompleted, nil
	}
	record = HookDelivery{
		DeliveryID: deliveryID,
		Hash:       hash,
		State:      hookDeliveryStatePending,
		CreatedAt:  now.UTC(),
		UpdatedAt:  now.UTC(),
	}
	return false, writeHookDelivery(dir, record, s.syncDir)
}

func (s *hookDeliveries) Complete(
	dir string,
	deliveryID string,
	hash string,
	now time.Time,
) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	record, found, err := readHookDelivery(dir, deliveryID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("agentjournal: hook delivery: pending record disappeared")
	}
	record.State = hookDeliveryStateCompleted
	record.UpdatedAt = now.UTC()
	if err := writeHookDelivery(dir, record, s.syncDir); err != nil {
		return err
	}
	s.markLocked(deliveryID, hash)
	s.maintainLocked(dir, now)
	return nil
}

func (s *hookDeliveries) markLocked(deliveryID, hash string) {
	if _, ok := s.completed[deliveryID]; ok {
		return
	}
	s.completed[deliveryID] = hash
	s.order = append(s.order, deliveryID)
	s.evictLocked()
}

func (s *hookDeliveries) evictLocked() {
	ids := s.order
	for len(ids) > HookDeliveryCompletedMax {
		delete(s.completed, ids[0])
		ids = ids[1:]
	}
	s.order = ids
}

func (s *hookDeliveries) maintainLocked(dir string, now time.Time) {
	s.completions++
	if s.completions%HookDeliveryPruneEvery != 0 {
		return
	}
	pruneHookDeliveries(dir)
	reapHookDeliveryRunners(filepath.Dir(dir), now)
}

func pruneHookDeliveries(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= HookDeliveryJournalMax {
		return
	}
	removeHookDeliveries(
		dir,
		completedHookDeliveries(dir, entries),
		len(entries)-HookDeliveryJournalMax,
	)
}

func completedHookDeliveries(
	dir string,
	entries []os.DirEntry,
) []HookDelivery {
	records := make([]HookDelivery, 0, min(len(entries), hookDeliveryScanMax))
	for _, entry := range entries[:min(len(entries), hookDeliveryScanMax)] {
		records = appendCompletedHookDelivery(records, dir, entry.Name())
	}
	sort.Slice(records, func(i, k int) bool {
		return records[i].UpdatedAt.Before(records[k].UpdatedAt)
	})
	return records
}

func appendCompletedHookDelivery(
	records []HookDelivery,
	dir string,
	name string,
) []HookDelivery {
	if !strings.HasSuffix(name, ".json") {
		return records
	}
	record, found, err := readHookDelivery(dir, strings.TrimSuffix(name, ".json"))
	if err != nil || !found || record.State != hookDeliveryStateCompleted {
		return records
	}
	return append(records, record)
}

func removeHookDeliveries(
	dir string,
	records []HookDelivery,
	excess int,
) {
	for _, record := range records {
		if excess <= 0 {
			return
		}
		if os.Remove(filepath.Join(dir, record.DeliveryID+".json")) == nil {
			excess--
		}
	}
}

func reapHookDeliveryRunners(root string, now time.Time) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	for _, entry := range entries[:min(len(entries), hookDeliveryScanMax)] {
		reapHookDeliveryRunner(root, entry, now)
	}
}

func reapHookDeliveryRunner(
	root string,
	entry os.DirEntry,
	now time.Time,
) {
	if !entry.IsDir() {
		return
	}
	info, err := entry.Info()
	if err != nil || now.Sub(info.ModTime()) <= HookDeliveryJournalMaxAge {
		return
	}
	_ = os.RemoveAll(filepath.Join(root, entry.Name()))
}

func readHookDelivery(dir, deliveryID string) (HookDelivery, bool, error) {
	var record HookDelivery
	found, err := readRecord(filepath.Join(dir, deliveryID+".json"), &record)
	if err != nil || !found {
		return HookDelivery{}, false, err
	}
	return record, true, nil
}

func writeHookDelivery(
	dir string,
	record HookDelivery,
	syncDir func(string) error,
) error {
	return writeRecord(dir, record.DeliveryID+".json", record, hookDeliveryTempPat, syncDir)
}
