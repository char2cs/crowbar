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

// State is why the daemon is not serving yet. Its spelling is the health
// route's status and, prefixed, the refusal's code.
type State string

const (
	// StateMigrating means the daemon is still preparing its home.
	StateMigrating State = "migrating"
	// StateHalted means the daemon refused, or failed, to prepare its home.
	StateHalted State = "halted"
)

// Report is the boot state the gate advertises. Message says why the daemon is
// not serving, and Detail is echoed on the health route.
type Report struct {
	State   State
	Message string
	Detail  any
}

// Reporter returns the current Report. The gate calls it on every request, so
// it must be safe for concurrent use.
type Reporter func() Report

// Middleware wraps the gate's own answers as the real API wraps its own, so a
// client reads them the same way.
type Middleware func(http.Handler) http.Handler

// Gate is an http.Handler that stands in front of the real API until Open.
// Build it with New.
type Gate struct {
	version string
	report  Reporter
	own     http.Handler
	real    atomic.Pointer[http.Handler]
}

// New returns a Gate that answers the health route from report and refuses
// everything else until Open is called, through mw. It panics if report or mw
// is nil.
func New(
	version string,
	report Reporter,
	mw Middleware,
) *Gate {
	if report == nil {
		panic("bootgate: New requires a Reporter")
	}
	if mw == nil {
		panic("bootgate: New requires a Middleware")
	}
	g := &Gate{version: version, report: report}
	g.own = mw(http.HandlerFunc(g.answer))
	return g
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
	g.own.ServeHTTP(w, r)
}

func (g *Gate) answer(
	w http.ResponseWriter,
	r *http.Request,
) {
	rep := g.report()
	if r.Method != http.MethodGet || r.URL.Path != healthPath {
		writeJSON(w, http.StatusServiceUnavailable, refusal{
			Success: false,
			Error:   rep.Message,
			Code:    refusalCodePrefix + string(rep.State),
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
	Status  State  `json:"status"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
	Detail  any    `json:"detail"`
}

type refusal struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code"`
}

// unencodable answers a request whose report cannot be encoded.
const unencodable = `{"success":false,"error":"bootgate: the boot report cannot be encoded","code":"daemon_report_unencodable"}`

// writeJSON encodes body before it commits to status, so an encode failure
// answers 500 instead of status with half a body.
func writeJSON(
	w http.ResponseWriter,
	status int,
	body any,
) {
	b, err := json.Marshal(body)
	if err != nil {
		slog.Error("bootgate: encode response", "status", status, "error", err)
		status, b = http.StatusInternalServerError, []byte(unencodable)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write(b); err != nil {
		slog.Debug("bootgate: write response: the client is gone", "status", status, "error", err)
	}
}
