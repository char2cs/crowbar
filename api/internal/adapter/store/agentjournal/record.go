package agentjournal

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const recordMode = 0o600

func writeRecord(
	dir string,
	name string,
	record any,
	tempPattern string,
	syncDir func(string) error,
) error {
	data, err := json.Marshal(record)
	if err != nil {
		return fmt.Errorf("agentjournal: encode %s: %w", name, err)
	}
	tmp, err := os.CreateTemp(dir, tempPattern)
	if err != nil {
		return fmt.Errorf("agentjournal: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if err := stageRecord(tmp, data); err != nil {
		return err
	}
	if err := os.Rename(tmpName, filepath.Join(dir, name)); err != nil {
		return fmt.Errorf("agentjournal: commit %s: %w", name, err)
	}
	return syncDir(dir)
}

func stageRecord(
	tmp *os.File,
	data []byte,
) error {
	if err := tmp.Chmod(recordMode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agentjournal: chmod temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agentjournal: write temp: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agentjournal: sync temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("agentjournal: close temp: %w", err)
	}
	return nil
}

func readRecord(
	path string,
	into any,
) (bool, error) {
	data, err := os.ReadFile(path) //nolint:gosec // path is daemon-generated under the chat journal
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("agentjournal: read %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(data, into); err != nil {
		return false, fmt.Errorf("agentjournal: decode %s: %w", filepath.Base(path), err)
	}
	return true, nil
}

func syncJournalDir(dir string) error {
	dh, err := os.Open(dir) //nolint:gosec // daemon-owned journal directory
	if err != nil {
		return fmt.Errorf("agentjournal: open dir for sync: %w", err)
	}
	syncErr := dh.Sync()
	closeErr := dh.Close()
	if syncErr != nil || closeErr != nil {
		return fmt.Errorf("agentjournal: sync dir: %w", errors.Join(syncErr, closeErr))
	}
	return nil
}
