//go:build unix

package descriptorcheck

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// recorder stands in for the daemon's hook intake. A CLI that runs a command is
// given a script standing in for `crowbar hook <event>`; one that posts its hooks
// itself (or through curl) is given a loopback server. Either writes the event
// name and its payload to a file of its own (hooks run concurrently, so one shared
// log would interleave).
type recorder struct {
	path   string
	dir    string
	url    string
	server *http.Server
}

// hookRecord is one hook the CLI fired.
type hookRecord struct {
	event   string
	payload []byte
}

func newRecorder(ctx context.Context, dir string) (recorder, error) {
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
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return recorder{}, fmt.Errorf("hook recorder: %w", err)
	}
	r.url = "http://" + ln.Addr().String()
	r.server = &http.Server{Handler: http.HandlerFunc(r.record), ReadHeaderTimeout: 10 * time.Second}
	go func() { _ = r.server.Serve(ln) }()
	return r, nil
}

// record keeps one posted hook: the event is the last path segment, as the
// daemon's runner-keyed hook route names it.
func (r recorder) record(
	w http.ResponseWriter,
	req *http.Request,
) {
	payload, err := io.ReadAll(req.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	f, err := os.CreateTemp(r.dir, "h.*.tmp")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	_, writeErr := fmt.Fprintf(f, "%s\n%s", path.Base(req.URL.Path), payload)
	if closeErr := f.Close(); writeErr != nil || closeErr != nil {
		http.Error(w, "record hook", http.StatusInternalServerError)
		return
	}
	if err := os.Rename(f.Name(), strings.TrimSuffix(f.Name(), ".tmp")+".rec"); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusOK)
}

// close stops the loopback server, waiting up to a second even when ctx is already
// cancelled. A recorder that never started one is a no-op.
func (r recorder) close(ctx context.Context) {
	if r.server == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), time.Second)
	defer cancel()
	if err := r.server.Shutdown(ctx); err != nil {
		slog.WarnContext(ctx, "descriptorcheck: stop the hook recorder", "err", err)
	}
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
