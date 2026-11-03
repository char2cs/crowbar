// Package bootgate answers the daemon's health route and refuses every other
// request until the real API is opened, so no handler runs over state that is
// still migrating or halted.
package bootgate

import (
	"encoding/json"
	"fmt"
	"html"
	"log/slog"
	"net/http"
	"os"
	"strings"
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
// that it answers GET /v0/health with the current Report, shows a browser a
// page that reloads once ready, and refuses every other request with a 503
// whose code names the state.
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
	if navigation(r) {
		writePage(w, rep)
		return
	}
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

// navigation reports whether r is a browser opening a page rather than a
// client calling the API, which keeps reading the refusal it acts on.
func navigation(
	r *http.Request,
) bool {
	if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/v0/") {
		return false
	}
	// Whichever of HTML and JSON is listed first wins; q-values are not weighed.
	accept := r.Header.Get("Accept")
	htmlAt, jsonAt := strings.Index(accept, "text/html"), strings.Index(accept, "application/json")
	return htmlAt >= 0 && (jsonAt < 0 || htmlAt < jsonAt)
}

// page waits on the health route, the one route the gate answers, and reloads
// once the real API answers it.
const page = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<title>Crowbar</title></head>
<body style="font:16px system-ui,sans-serif;max-width:32rem;margin:20vh auto;padding:0 16px">
<p>%s</p><p style="opacity:.6">%s</p><p>%s</p>
<script>setInterval(() => fetch("/v0/health").then((r) => r.json())
  .then((b) => { if (b.data && b.data.status === "ok") location.reload() }).catch(() => {}), 1000)</script>
</body></html>`

// pageWords are what each state means to someone who opened the page, before
// and after its message: a halt ends only when someone restarts Crowbar.
var pageWords = map[State][2]string{
	StateMigrating: {"Crowbar is updating your data… this page reloads by itself.", ""},
	StateHalted: {
		"Crowbar could not start.",
		"Fix the cause above and restart Crowbar. This page will reload once it is running.",
	},
}

// writePage answers a browser with the page for rep.
func writePage(
	w http.ResponseWriter,
	rep Report,
) {
	words := pageWords[rep.State]
	body := fmt.Sprintf(page, html.EscapeString(words[0]), html.EscapeString(rep.Message), html.EscapeString(words[1]))
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusServiceUnavailable)
	if _, err := w.Write([]byte(body)); err != nil {
		slog.Debug("bootgate: write page: the client is gone", "error", err)
	}
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
