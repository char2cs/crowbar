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

// MaxPayloadBytes is the most of one payload a blob keeps.
const MaxPayloadBytes = 8 << 20

const truncationMarker = "\n\n[crowbar: payload truncated at 8 MiB]"

// RefPrefix starts every blob ref: the rest is the payload's sha256.
const RefPrefix = "sha256:"

// ErrBlobNotFound is a ref no blob is stored under.
var ErrBlobNotFound = errors.New("ledger blobs: not found")

// Blobs stores tool payloads by content: a row carries only a payload's ref.
type Blobs struct {
	root string
	// refs is held shared while payloads are named, from Put until the row
	// naming them commits, and alone by a collection.
	refs sync.RWMutex
}

// OpenBlobs is the blob store under root, created if it is missing.
func OpenBlobs(root string) (*Blobs, error) {
	if root == "" {
		return nil, errors.New("ledger blobs: empty root")
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("ledger blobs: mkdir: %w", err)
	}
	return &Blobs{root: root}, nil
}

// Put stores data and returns its ref; "" for an empty payload. The same
// payload is stored once, and one past MaxPayloadBytes is cut and says so.
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

// Get is the payload stored under ref.
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

// Naming holds collection off until release: Put the payloads and commit the
// rows naming them before releasing.
func (s *Blobs) Naming() (release func()) {
	s.refs.RLock()
	return s.refs.RUnlock
}

// Collect deletes the blob behind ref unless inUse says a row names it, with
// no payload being named meanwhile. An empty, malformed or missing ref is a
// no-op, so two purges racing over one shared blob both succeed.
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
	path, _ := s.pathFor(ref)
	if path == "" {
		return nil
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("ledger blobs: delete: %w", err)
	}
	return nil
}

func (s *Blobs) pathFor(ref string) (path, dir string) {
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
		return fmt.Errorf("ledger blobs: temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		if err != nil {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err = tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ledger blobs: write: %w", err)
	}
	if err = tmp.Sync(); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("ledger blobs: sync: %w", err)
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

func syncDir(dir string) error {
	dh, err := os.Open(dir) //nolint:gosec // daemon-owned content directory
	if err != nil {
		return fmt.Errorf("ledger blobs: open dir: %w", err)
	}
	defer func() { _ = dh.Close() }()
	if err := dh.Sync(); err != nil {
		return fmt.Errorf("ledger blobs: sync dir: %w", err)
	}
	return nil
}
