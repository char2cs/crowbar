// Package bootgate answers the daemon's health route and refuses every other
// request until the real API is opened, so no handler runs over state that is
// still migrating or halted.
package bootgate

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"sync/atomic"
)

const (
	healthPath        = "/v0/health"
	refusalCodePrefix = "daemon_"
)

// Report is the boot state the gate advertises. State is "migrating" or
// "halted", Message says why the daemon is not serving, and Detail is echoed
// on the health route.
type Report struct {
	State   string
	Message string
	Detail  any
}

// Reporter returns the current Report. The gate calls it on every request, so
// it must be safe for concurrent use.
type Reporter func() Report

// Gate is an http.Handler that stands in front of the real API until Open.
// Build it with New.
type Gate struct {
	version string
	report  Reporter
	real    atomic.Pointer[http.Handler]
}

// New returns a Gate that answers the health route from report and refuses
// everything else until Open is called. It panics if report is nil.
func New(
	version string,
	report Reporter,
) *Gate {
	if report == nil {
		panic("bootgate: New requires a Reporter")
	}
	return &Gate{version: version, report: report}
}

// Open routes every request that arrives after it to h. It is safe to call
// while requests are in flight, and it panics if h is nil.
func (g *Gate) Open(
	h http.Handler,
) {
	if h == nil {
		panic("bootgate: Open requires a handler")
	}
	g.real.Store(&h)
}

// ServeHTTP hands the request to the real handler once Open has run. Before
// that it answers GET /v0/health with the current Report and refuses every
// other request with a 503 whose code names the state.
func (g *Gate) ServeHTTP(
	w http.ResponseWriter,
	r *http.Request,
) {
	if h := g.real.Load(); h != nil {
		(*h).ServeHTTP(w, r)
		return
	}
	rep := g.report()
	if r.Method != http.MethodGet || r.URL.Path != healthPath {
		writeJSON(w, http.StatusServiceUnavailable, refusal{
			Success: false,
			Error:   rep.Message,
			Code:    refusalCodePrefix + rep.State,
		})
		return
	}
	writeJSON(w, http.StatusOK, healthBody{
		Success: true,
		Data: healthData{
			Status:  rep.State,
			Version: g.version,
			PID:     os.Getpid(),
			Detail:  rep.Detail,
		},
	})
}

type healthBody struct {
	Success bool       `json:"success"`
	Data    healthData `json:"data"`
}

type healthData struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
	Detail  any    `json:"detail"`
}

type refusal struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code"`
}

func writeJSON(
	w http.ResponseWriter,
	status int,
	body any,
) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Error("bootgate: write response", "status", status, "error", err)
	}
}
