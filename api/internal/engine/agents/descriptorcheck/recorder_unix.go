//go:build unix

package descriptorcheck

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// recorder stands in for `crowbar hook <event>`: a script the CLI runs for
// every hook, writing the event name and its stdin payload to a file of its
// own (hooks run concurrently, so one shared log would interleave).
type recorder struct {
	path string
	dir  string
}

// hookRecord is one hook the CLI fired.
type hookRecord struct {
	event   string
	payload []byte
}

func newRecorder(dir string) (recorder, error) {
	r := recorder{path: filepath.Join(dir, "hook"), dir: filepath.Join(dir, "hooks")}
	if err := os.MkdirAll(r.dir, 0o750); err != nil {
		return recorder{}, fmt.Errorf("hook recorder: %w", err)
	}
	// Only `hook` is recorded: the MCP server the descriptor also points at
	// this path must exit at once rather than read a stdin that never closes.
	script := fmt.Sprintf("#!/bin/sh\n[ \"$1\" = hook ] || exit 1\nf=$(mktemp '%s/h.XXXXXXXX') || exit 1\n{ printf '%%s\\n' \"$2\"; cat; } > \"$f.tmp\" && mv \"$f.tmp\" \"$f.rec\"\n",
		strings.ReplaceAll(r.dir, "'", `'\''`))
	if err := os.WriteFile(r.path, []byte(script), 0o700); err != nil { //nolint:gosec // the recorder must be executable
		return recorder{}, fmt.Errorf("hook recorder: %w", err)
	}
	return r, nil
}

// records is every completed hook, oldest first.
func (r recorder) records() []hookRecord {
	files, _ := filepath.Glob(filepath.Join(r.dir, "*.rec"))
	type stamped struct {
		rec hookRecord
		at  int64
	}
	var all []stamped
	for _, f := range files {
		raw, err := os.ReadFile(f) //nolint:gosec // a file this run's recorder wrote
		info, statErr := os.Stat(f)
		if err != nil || statErr != nil {
			continue
		}
		event, payload, _ := strings.Cut(string(raw), "\n")
		all = append(all, stamped{hookRecord{event: event, payload: bytes.TrimSpace([]byte(payload))}, info.ModTime().UnixNano()})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].at < all[j].at })
	out := make([]hookRecord, len(all))
	for i, s := range all {
		out[i] = s.rec
	}
	return out
}

func (r recorder) has(event string) bool {
	for _, rec := range r.records() {
		if rec.event == event {
			return true
		}
	}
	return false
}
