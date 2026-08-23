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

func deliverHookEnvelope(
	ctx context.Context,
	host string,
	envelope hookEnvelope,
) ([]byte, error) {
	client, err := ipc.NewClient(host)
	if err != nil {
		return nil, err
	}
	status, body, err := client.PostJSON(
		ctx,
		scopedAgentPath(envelope.Project, envelope.Repo, envelope.Workspace, "/hooks"),
		envelope,
	)
	if err != nil {
		return nil, err
	}
	if status < http.StatusOK || status >= http.StatusMultipleChoices {
		msg := strings.TrimSpace(string(body))
		if len(msg) > 512 {
			msg = msg[:512]
		}
		return nil, fmt.Errorf("hook daemon returned HTTP %d: %s", status, msg)
	}
	return body, nil
}

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
	_, err := drainHookSpoolFor(ctx, host, "")
	return err
}

func drainHookSpoolFor(
	ctx context.Context,
	host string,
	deliveryID string,
) (mine []byte, err error) {
	dir := hookSpoolDir()
	entries, err := os.ReadDir(dir)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("hook spool: read: %w", err)
	}
	release, acquired, err := acquireHookDrain(dir)
	if err != nil || !acquired {
		return nil, err
	}
	defer release()

	for _, name := range spooledNames(entries) {
		if err := os.Chtimes(filepath.Join(dir, hookDrainLockName), time.Now(), time.Now()); err != nil {
			return mine, fmt.Errorf("hook spool: renew drain lease: %w", err)
		}
		envelope, body, err := deliverSpooled(ctx, host, dir, name)
		if err != nil {
			return mine, err
		}
		if deliveryID != "" && envelope.DeliveryID == deliveryID {
			mine = body
		}
	}
	return mine, nil
}

func spooledNames(entries []os.DirEntry) []string {
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".json" {
			names = append(names, entry.Name())
		}
	}
	sort.Strings(names)
	return names
}

func deliverSpooled(
	ctx context.Context,
	host, dir, name string,
) (envelope hookEnvelope, body []byte, err error) {
	path := filepath.Join(dir, name)
	data, err := os.ReadFile(path) //nolint:gosec // listed from Crowbar-owned spool
	if errors.Is(err, os.ErrNotExist) {

		return hookEnvelope{}, nil, nil
	}
	if err != nil {
		return hookEnvelope{}, nil, fmt.Errorf("hook spool: read %s: %w", name, err)
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return hookEnvelope{}, nil, fmt.Errorf("hook spool: decode %s: %w", name, err)
	}
	body, err = deliverHookEnvelope(ctx, host, envelope)
	if err != nil {
		return hookEnvelope{}, nil, err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return envelope, body, fmt.Errorf("hook spool: remove acknowledged %s: %w", name, err)
	}
	if err := syncHookSpoolDir(dir); err != nil {
		return envelope, body, err
	}
	return envelope, body, nil
}

func drainHookSpoolLoop(ctx context.Context, host string) {

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
