package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/ipc"
	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

const (
	hookSpoolDirName   = "hook-spool"
	hookDrainLockName  = ".drain-lock"
	hookDrainLockStale = 30 * time.Second
)

// hookEnvelope is entirely Crowbar-owned. Providers supply only payload_raw;
// the relay mints delivery_id before its first POST and preserves the whole
// envelope until the daemon acknowledges it with a 2xx response.
type hookEnvelope struct {
	DeliveryID string `json:"delivery_id"`
	SegmentID  string `json:"segment_id"`
	Provider   string `json:"provider"`
	Event      string `json:"event"`
	PayloadRaw string `json:"payload_raw"`
	Project    string `json:"project"`
	Repo       string `json:"repo"`
	Workspace  string `json:"workspace"`
	CreatedAt  string `json:"created_at"`
}

func hookSpoolDir() string {
	return filepath.Join(metadata.GetHomePath(), hookSpoolDirName)
}

func persistHookEnvelope(envelope hookEnvelope) (string, error) {
	dir := hookSpoolDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("hook spool: mkdir: %w", err)
	}
	data, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("hook spool: encode: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".hook-*")
	if err != nil {
		return "", fmt.Errorf("hook spool: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := tmp.Chmod(0o600); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("hook spool: chmod: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("hook spool: write: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return "", fmt.Errorf("hook spool: sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return "", fmt.Errorf("hook spool: close: %w", err)
	}
	created, err := time.Parse(time.RFC3339Nano, envelope.CreatedAt)
	if err != nil {
		return "", fmt.Errorf("hook spool: created_at: %w", err)
	}
	name := fmt.Sprintf("%020d-%s.json", created.UnixNano(), envelope.DeliveryID)
	path := filepath.Join(dir, name)
	if err := os.Rename(tmpName, path); err != nil {
		return "", fmt.Errorf("hook spool: commit: %w", err)
	}
	if err := syncHookSpoolDir(dir); err != nil {
		return "", err
	}
	return path, nil
}

func syncHookSpoolDir(dir string) error {
	dh, err := os.Open(dir) //nolint:gosec // Crowbar-owned spool
	if err != nil {
		return fmt.Errorf("hook spool: open parent for sync: %w", err)
	}
	syncErr := dh.Sync()
	closeErr := dh.Close()
	if syncErr != nil || closeErr != nil {
		return fmt.Errorf("hook spool: sync parent: %w", errors.Join(syncErr, closeErr))
	}
	return nil
}

func deliverHookEnvelope(ctx context.Context, host string, envelope hookEnvelope) error {
	client, err := ipc.NewClient(host)
	if err != nil {
		return err
	}
	status, body, err := client.PostJSON(
		ctx,
		scopedAgentPath(envelope.Project, envelope.Repo, envelope.Workspace, "/hooks"),
		envelope,
	)
	if err != nil {
		return err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 512 {
			msg = msg[:512]
		}
		return fmt.Errorf("hook daemon returned HTTP %d: %s", status, msg)
	}
	return nil
}

// acquireHookDrain uses an atomic directory as a cross-process lease. Hook
// callbacks are separate short-lived processes, while the daemon also drains
// periodically; exactly one of them may walk the ordered spool at a time.
func acquireHookDrain(dir string) (release func(), acquired bool, err error) {
	lock := filepath.Join(dir, hookDrainLockName)
	if err := os.Mkdir(lock, 0o700); err == nil {
		return func() { _ = os.Remove(lock) }, true, nil
	} else if !errors.Is(err, os.ErrExist) {
		return nil, false, fmt.Errorf("hook spool: acquire drain lease: %w", err)
	}
	info, statErr := os.Stat(lock)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			return acquireHookDrain(dir)
		}
		return nil, false, fmt.Errorf("hook spool: inspect drain lease: %w", statErr)
	}
	if time.Since(info.ModTime()) <= hookDrainLockStale {
		return nil, false, nil
	}
	if err := os.Remove(lock); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("hook spool: recover stale drain lease: %w", err)
	}
	return acquireHookDrain(dir)
}

func drainHookSpool(ctx context.Context, host string) error {
	dir := hookSpoolDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("hook spool: read: %w", err)
	}
	release, acquired, err := acquireHookDrain(dir)
	if err != nil || !acquired {
		return err
	}
	defer release()

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	for _, name := range names {
		if err := os.Chtimes(filepath.Join(dir, hookDrainLockName), time.Now(), time.Now()); err != nil {
			return fmt.Errorf("hook spool: renew drain lease: %w", err)
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path) //nolint:gosec // listed from Crowbar-owned spool
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("hook spool: read %s: %w", name, err)
		}
		var envelope hookEnvelope
		if err := json.Unmarshal(data, &envelope); err != nil {
			return fmt.Errorf("hook spool: decode %s: %w", name, err)
		}
		if err := deliverHookEnvelope(ctx, host, envelope); err != nil {
			return err // preserve FIFO: never overtake an undelivered older hook
		}
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("hook spool: remove acknowledged %s: %w", name, err)
		}
		if err := syncHookSpoolDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func drainHookSpoolLoop(ctx context.Context, host string) {
	// The listener starts immediately after this goroutine. A first failed pass
	// is ordinary; the ticker retries without discarding anything.
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		if err := drainHookSpool(ctx, host); err != nil && ctx.Err() == nil {
			slog.DebugContext(ctx, "crowbar hook spool: delivery deferred", "err", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
