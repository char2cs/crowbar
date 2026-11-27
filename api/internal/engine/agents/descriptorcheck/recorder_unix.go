//go:build unix

package descriptorcheck

import (
	"bytes"
	"context"
	"errors"
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

type recorder struct {
	path   string
	dir    string
	url    string
	server *http.Server
}

type hookRecord struct {
	event   string
	payload []byte
}

func newRecorder(ctx context.Context, dir string) (recorder, error) {
	r := recorder{path: filepath.Join(dir, "hook"), dir: filepath.Join(dir, "hooks")}
	if err := os.MkdirAll(r.dir, 0o750); err != nil {
		return recorder{}, fmt.Errorf("hook recorder: %w", err)
	}
	if err := os.WriteFile(r.path, []byte("#!/bin/sh\nexit 1\n"), 0o700); err != nil { //nolint:gosec // the stand-in must be executable
		return recorder{}, fmt.Errorf("hook recorder: %w", err)
	}
	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return recorder{}, fmt.Errorf("hook recorder: %w", err)
	}
	r.url = "http://" + ln.Addr().String()
	r.server = &http.Server{Handler: http.HandlerFunc(r.record), ReadHeaderTimeout: 10 * time.Second}
	go func() {
		if err := r.server.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
			slog.Warn("descriptorcheck: the hook recorder stopped", "err", err)
		}
	}()
	return r, nil
}

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

func (r recorder) records() []hookRecord {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		slog.Warn("descriptorcheck: read the hook records", "dir", r.dir, "err", err)
		return nil
	}
	type stamped struct {
		rec hookRecord
		at  int64
	}
	var all []stamped
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".rec") {
			continue
		}
		f := filepath.Join(r.dir, e.Name())
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
