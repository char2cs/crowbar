package ledger

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const MaxPayloadBytes = 8 << 20

const truncationMarker = "\n\n[crowbar: payload truncated at 8 MiB]"

const RefPrefix = "sha256:"

var ErrBlobNotFound = errors.New("ledger blobs: not found")

type Blobs struct {
	root string
	refs sync.RWMutex
}

func OpenBlobs(root string) (*Blobs, error) {
	if root == "" {
		return nil, errors.New("ledger blobs: empty root")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("ledger blobs: mkdir: %w", err)
	}
	return &Blobs{root: root}, nil
}

func (s *Blobs) Put(data []byte) (string, error) {
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
		return "", fmt.Errorf("ledger blobs: mkdir: %w", err)
	}
	if err := writeDurable(path, data); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *Blobs) Get(ref string) ([]byte, error) {
	if ref == "" {
		return nil, ErrBlobNotFound
	}
	path, _ := s.pathFor(ref)
	if path == "" {
		return nil, ErrBlobNotFound
	}
	data, err := os.ReadFile(path) //nolint:gosec // path is derived from a hex digest under a daemon-owned root
	if errors.Is(err, os.ErrNotExist) {
		return nil, ErrBlobNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("ledger blobs: read: %w", err)
	}
	return data, nil
}

func (s *Blobs) Naming() (release func()) {
	s.refs.RLock()
	return s.refs.RUnlock
}

func (s *Blobs) Collect(ref string, inUse func(string) (bool, error)) error {
	s.refs.Lock()
	defer s.refs.Unlock()
	used, err := inUse(ref)
	if err != nil || used {
		return err
	}
	return s.remove(ref)
}

func (s *Blobs) remove(ref string) error {
	path, dir := s.pathFor(ref)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("ledger blobs: delete: %w", err)
	}
	s.pruneShard(dir)
	return nil
}

func (s *Blobs) pruneShard(dir string) {
	for _, d := range []string{dir, filepath.Dir(dir)} {
		if d == s.root || os.Remove(d) != nil {
			return
		}
	}
}

func ValidRef(ref string) bool {
	digest, ok := strings.CutPrefix(ref, RefPrefix)
	return ok && len(digest) == sha256.Size*2 && isHex(digest)
}

func (s *Blobs) pathFor(ref string) (path, dir string) {
	if !ValidRef(ref) {
		return "", ""
	}
	digest := ref[len(RefPrefix):]
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
		return fmt.Errorf("ledger blobs: temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			err = errors.Join(err, os.Remove(tmpName))
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		return errors.Join(fmt.Errorf("ledger blobs: write: %w", err), tmp.Close())
	}
	if err = tmp.Sync(); err != nil {
		return errors.Join(fmt.Errorf("ledger blobs: sync: %w", err), tmp.Close())
	}
	if err = tmp.Close(); err != nil {
		return fmt.Errorf("ledger blobs: close: %w", err)
	}
	if err = os.Chmod(tmpName, 0o600); err != nil {
		return fmt.Errorf("ledger blobs: chmod: %w", err)
	}
	if err = os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("ledger blobs: rename: %w", err)
	}

	return syncDir(dir)
}

func syncDir(dir string) (err error) {
	dh, err := os.Open(dir) //nolint:gosec // daemon-owned content directory
	if err != nil {
		return fmt.Errorf("ledger blobs: open dir: %w", err)
	}
	defer func() { err = errors.Join(err, dh.Close()) }()
	if err := dh.Sync(); err != nil {
		return fmt.Errorf("ledger blobs: sync dir: %w", err)
	}
	return nil
}
