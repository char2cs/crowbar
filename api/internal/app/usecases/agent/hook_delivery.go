package agent

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
)

const (
	hookDeliveryDirName = ".hook-deliveries"

	hookDeliveryStatePending   = "pending"
	hookDeliveryStateCompleted = "completed"

	hookDeliveryCompletedMax  = 32
	hookDeliveryJournalMax    = 128
	hookDeliveryJournalMaxAge = 30 * 24 * time.Hour
	hookDeliveryPruneEvery    = 16
	hookDeliveryScanMax       = 1024
)

type hookDeliveryContextKey struct{}

func hookDeliveryID(ctx context.Context) string {
	id, _ := ctx.Value(hookDeliveryContextKey{}).(string)
	return id
}

type hookDeliveryRecord struct {
	DeliveryID string    `json:"deliveryId"`
	Hash       string    `json:"hash"`
	State      string    `json:"state"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
}

type hookDeliveryJournal struct {
	mu          sync.Mutex
	gates       *chatGate
	syncDir     func(string) error
	completed   map[string]string
	order       []string
	completions int
}

func newHookDeliveryJournal() *hookDeliveryJournal {
	return &hookDeliveryJournal{
		gates:     newChatGate(),
		syncDir:   syncPromptJournalDir,
		completed: map[string]string{},
	}
}

func hookDeliveryHash(runnerID, provider, event string, raw []byte) string {
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

func (j *hookDeliveryJournal) begin(
	dir, deliveryID, hash string,
	now time.Time,
) (done bool, err error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if completedHash, ok := j.completed[deliveryID]; ok {
		if completedHash != hash {
			return false, fmt.Errorf("agent: hook delivery id reused with different payload")
		}
		return true, nil
	}

	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, fmt.Errorf("agent: hook delivery journal: mkdir: %w", err)
	}
	record, found, err := readHookDelivery(dir, deliveryID)
	if err != nil {
		return false, err
	}
	if found {
		if record.Hash != hash {
			return false, fmt.Errorf("agent: hook delivery id reused with different payload")
		}
		return record.State == hookDeliveryStateCompleted, nil
	}
	record = hookDeliveryRecord{
		DeliveryID: deliveryID,
		Hash:       hash,
		State:      hookDeliveryStatePending,
		CreatedAt:  now.UTC(),
		UpdatedAt:  now.UTC(),
	}
	return false, writeHookDelivery(dir, record, j.syncDir)
}

func (j *hookDeliveryJournal) complete(
	dir, deliveryID, hash string,
	now time.Time,
) error {
	j.mu.Lock()
	defer j.mu.Unlock()

	record, found, err := readHookDelivery(dir, deliveryID)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("agent: hook delivery journal: pending record disappeared")
	}
	record.State = hookDeliveryStateCompleted
	record.UpdatedAt = now.UTC()
	if err := writeHookDelivery(dir, record, j.syncDir); err != nil {
		return err
	}
	j.markLocked(deliveryID, hash)
	j.maintainLocked(dir, now)
	return nil
}

func (j *hookDeliveryJournal) markLocked(deliveryID, hash string) {
	if _, ok := j.completed[deliveryID]; ok {
		return
	}
	j.completed[deliveryID] = hash
	j.order = append(j.order, deliveryID)
	j.evictLocked()
}

func (j *hookDeliveryJournal) evictLocked() {
	ids := j.order
	for len(ids) > hookDeliveryCompletedMax {
		delete(j.completed, ids[0])
		ids = ids[1:]
	}
	j.order = ids
}

func (j *hookDeliveryJournal) maintainLocked(dir string, now time.Time) {
	j.completions++
	if j.completions%hookDeliveryPruneEvery != 0 {
		return
	}
	pruneHookDeliveries(dir)
	reapHookDeliveryRunners(filepath.Dir(dir), now)
}

func pruneHookDeliveries(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) <= hookDeliveryJournalMax {
		return
	}
	removeHookDeliveries(
		dir,
		completedHookDeliveries(dir, entries),
		len(entries)-hookDeliveryJournalMax,
	)
}

func completedHookDeliveries(
	dir string,
	entries []os.DirEntry,
) []hookDeliveryRecord {
	records := make([]hookDeliveryRecord, 0, min(len(entries), hookDeliveryScanMax))
	for _, entry := range entries[:min(len(entries), hookDeliveryScanMax)] {
		records = appendCompletedHookDelivery(records, dir, entry.Name())
	}
	sort.Slice(records, func(i, k int) bool {
		return records[i].UpdatedAt.Before(records[k].UpdatedAt)
	})
	return records
}

func appendCompletedHookDelivery(
	records []hookDeliveryRecord,
	dir string,
	name string,
) []hookDeliveryRecord {
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
	records []hookDeliveryRecord,
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
	if err != nil || now.Sub(info.ModTime()) <= hookDeliveryJournalMaxAge {
		return
	}
	_ = os.RemoveAll(filepath.Join(root, entry.Name()))
}

func readHookDelivery(dir, deliveryID string) (hookDeliveryRecord, bool, error) {
	data, err := os.ReadFile(filepath.Join(dir, deliveryID+".json")) //nolint:gosec // canonical UUID under daemon-owned dir
	if errors.Is(err, os.ErrNotExist) {
		return hookDeliveryRecord{}, false, nil
	}
	if err != nil {
		return hookDeliveryRecord{}, false, fmt.Errorf("agent: hook delivery journal: read: %w", err)
	}
	var record hookDeliveryRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return hookDeliveryRecord{}, false, fmt.Errorf("agent: hook delivery journal: decode: %w", err)
	}
	return record, true, nil
}

func writeHookDelivery(
	dir string,
	record hookDeliveryRecord,
	syncDir func(string) error,
) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("agent: hook delivery journal: encode: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".hook-delivery-*")
	if err != nil {
		return fmt.Errorf("agent: hook delivery journal: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: hook delivery journal: chmod: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: hook delivery journal: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agent: hook delivery journal: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("agent: hook delivery journal: close: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(dir, record.DeliveryID+".json")); err != nil {
		return fmt.Errorf("agent: hook delivery journal: commit: %w", err)
	}
	if err := syncDir(dir); err != nil {
		return fmt.Errorf("agent: hook delivery journal: %w", err)
	}
	return nil
}

func (u *Usecase) IngestHookDelivery(
	ctx context.Context,
	workspaceID, deliveryID, runnerID, provider, canonicalEvent string,
	rawPayload []byte,
) error {
	parsed, err := uuid.Parse(deliveryID)
	if err != nil || parsed.String() != deliveryID {
		return fmt.Errorf("agent: hook delivery id must be a canonical UUID")
	}
	if workspaceID == "" {
		runner, getErr := u.runners.Get(ctx, runnerID)
		if errors.Is(getErr, agentrunner.ErrNotFound) {
			return nil
		}
		if getErr != nil {
			return fmt.Errorf("agent: hook delivery: runner: %w", getErr)
		}
		workspaceID = runner.WorkspaceID
	}
	chatsDir, err := u.ws.AgentChatsDir(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("agent: hook delivery: chats dir: %w", err)
	}
	dir := filepath.Join(chatsDir, hookDeliveryDirName, runnerID)
	hash := hookDeliveryHash(runnerID, provider, canonicalEvent, rawPayload)

	defer u.hookDeliveries.gates.lock(runnerID)()
	done, err := u.hookDeliveries.begin(dir, deliveryID, hash, time.Now())
	if err != nil || done {
		return err
	}

	if handled, enqueueErr := u.pendingHooks.enqueueDelivery(
		runnerID, provider, canonicalEvent, rawPayload, deliveryID, dir, hash,
	); handled {
		return enqueueErr
	}

	deliveryCtx := context.WithValue(ctx, hookDeliveryContextKey{}, deliveryID)
	if err := u.ingestHookNow(deliveryCtx, runnerID, provider, canonicalEvent, rawPayload); err != nil {
		return err
	}
	if err := u.hookDeliveries.complete(dir, deliveryID, hash, time.Now()); err != nil {
		slog.ErrorContext(ctx, "agent: persist completed hook delivery (effects already committed)",
			"runner_id", runnerID, "delivery_id", deliveryID, "err", err)
	}
	return nil
}
