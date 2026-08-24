package content

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const MaxPayloadBytes = 8 << 20

const truncationMarker = "\n\n[crowbar: payload truncated at 8 MiB]"

const RefPrefix = "sha256:"

var ErrNotFound = errors.New("agentactivity content: not found")

type Store struct {
	root string
}

func New(root string) (*Store, error) {
	if root == "" {
		return nil, fmt.Errorf("agentactivity content: empty root")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("agentactivity content: mkdir: %w", err)
	}
	return &Store{root: root}, nil
}

func (s *Store) Put(data []byte) (string, error) {
	if len(data) == 0 {
		return "", nil
	}
	if len(data) > MaxPayloadBytes {
		data = append(append([]byte{}, data[:MaxPayloadBytes]...), truncationMarker...)
	}
	sum := sha256.Sum256(data)
	ref := RefPrefix + hex.EncodeToString(sum[:])

	path, dir := s.pathFor(ref)
	if _, err := os.Stat(path); err == nil {
		return ref, nil
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", fmt.Errorf("agentactivity content: mkdir: %w", err)
	}
	if err := writeDurable(path, data); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *Store) Get(ref string) ([]byte, error) {
	if ref == "" {
		return nil, ErrNotFound
	}
	path, _ := s.pathFor(ref)
	if path == "" {
		return nil, ErrNotFound
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from a hex digest under a daemon-owned root
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("agentactivity content: read: %w", err)
	}
	return data, nil
}

func (s *Store) pathFor(ref string) (path, dir string) {
	digest, ok := strings.CutPrefix(ref, RefPrefix)
	if !ok || len(digest) != sha256.Size*2 || !isHex(digest) {
		return "", ""
	}
	dir = filepath.Join(s.root, digest[:2], digest[2:4])
	return filepath.Join(dir, digest), dir
}

func isHex(s string) bool {
	for _, r := range s {
		switch {
		case r >= '0' && r <= '9', r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return true
}

func writeDurable(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("agentactivity content: temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agentactivity content: write: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("agentactivity content: sync: %w", err)
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("agentactivity content: close: %w", err)
	}
	if err = os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("agentactivity content: chmod: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("agentactivity content: rename: %w", err)
	}

	return syncDir(dir)
}

func syncDir(dir string) error {
	dh, err := os.Open(dir) //nolint:gosec // daemon-owned content directory
	if err != nil {
		return fmt.Errorf("agentactivity content: open dir: %w", err)
	}
	defer func() { _ = dh.Close() }()
	if err := dh.Sync(); err != nil {
		return fmt.Errorf("agentactivity content: sync dir: %w", err)
	}
	return nil
}
