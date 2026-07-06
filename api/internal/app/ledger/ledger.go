// Package ledger implements the per-chat, append-only, provider-tagged store
// of opaque transcript snapshots. Crowbar owns this store; the contents are
// never parsed (agentic-engine spec §6).
package ledger

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// Ledger is a per-chat, append-only, provider-tagged store of opaque
// transcript snapshots.
type Ledger struct{ dir string }

// Open ensures the ledger directory exists and returns a handle to it.
func Open(dir string) (*Ledger, error) {
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return nil, fmt.Errorf("ledger: mkdir: %w", err)
	}
	return &Ledger{dir: dir}, nil
}

// Append writes the next opaque snapshot, prefixed with a zero-padded
// sequence so lexical order == chronological order. Returns the written
// filename.
func (l *Ledger) Append(providerID string, at time.Time, blob []byte) (string, error) {
	seq, err := l.nextSeq()
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("%04d-%s-%s.blob", seq, at.UTC().Format("20060102T150405Z"), providerID)
	if err := os.WriteFile(filepath.Join(l.dir, name), blob, 0o640); err != nil { //nolint:gosec // ledger entries are group-readable by design; name is ledger-generated
		return "", fmt.Errorf("ledger: write: %w", err)
	}
	return name, nil
}

func (l *Ledger) entries() ([]string, error) {
	des, err := os.ReadDir(l.dir)
	if err != nil {
		return nil, fmt.Errorf("ledger: readdir: %w", err)
	}
	var names []string
	for _, de := range des {
		if !de.IsDir() && filepath.Ext(de.Name()) == ".blob" {
			names = append(names, de.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (l *Ledger) nextSeq() (int, error) {
	names, err := l.entries()
	if err != nil {
		return 0, err
	}
	return len(names) + 1, nil
}

// ReadAll concatenates every entry in order, separated by a legible header so
// a receiving model can tell segments apart.
func (l *Ledger) ReadAll() ([]byte, error) {
	names, err := l.entries()
	if err != nil {
		return nil, err
	}
	var out []byte
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(l.dir, n)) //nolint:gosec // n comes from entries() listing l.dir, not external input
		if err != nil {
			return nil, fmt.Errorf("ledger: read %s: %w", n, err)
		}
		out = append(out, []byte("\n===== LEDGER ENTRY "+n+" =====\n")...)
		out = append(out, data...)
	}
	return out, nil
}
