// Package content stores tool payloads by content hash.
//
// It exists because the payloads must be kept in full and must not go anywhere
// near the event log: an aggregate holding them would have every snapshot rewrite
// every payload in that chat, and every cold load materialise all of them.
//
// Content addressing is not incidental. Agents re-read the same files constantly,
// so the same 200 KB file read forty times is stored once — and retention becomes
// a policy over this directory rather than a property of history.
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

// MaxPayloadBytes bounds a single stored payload.
//
// A provider is not adversarial, but it is also not bounded: a tool that dumps a
// large binary would otherwise write it verbatim, once per invocation, forever.
// Truncation is marked in the stored bytes so a reader is never shown a partial
// payload that looks whole.
const MaxPayloadBytes = 8 << 20

const truncationMarker = "\n\n[crowbar: payload truncated at 8 MiB]"

// RefPrefix namespaces a ref so a future digest change is distinguishable rather
// than silently incompatible.
const RefPrefix = "sha256:"

// ErrNotFound reports a ref with no stored payload. It is an ordinary outcome —
// retention may have swept it — so callers render "payload no longer available"
// rather than failing.
var ErrNotFound = errors.New("agentactivity content: not found")

// Store is a content-addressed blob directory.
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

// Put stores data and returns its ref. Empty data has no ref: there is nothing to
// address, and a ref that resolves to nothing is worse than no ref.
//
// Writing is fsync, atomic rename, fsync of the parent directory — the same
// discipline the flat-file conversation ledger used, kept because the property it
// bought is still required: an acknowledged hook must survive an OS crash.
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
		// Already stored. Content addressing means an identical payload is
		// byte-identical, so there is nothing to rewrite.
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

// Get returns a stored payload.
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

// pathFor derives a payload's location. The two-level fan-out keeps any single
// directory small enough that a filesystem listing stays usable.
//
// It returns empty for a ref that is not a well-formed digest, which is what stops
// a stored value from being read as a path.
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
	// Renaming makes the file visible; only fsyncing the DIRECTORY makes that
	// visibility survive a power loss.
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
