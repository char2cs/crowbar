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

type State string

const (
	StateMigrating State = "migrating"
	StateHalted    State = "halted"
)

type Report struct {
	State   State
	Message string
	Detail  any
}

type Reporter func() Report

type Middleware func(http.Handler) http.Handler

type Gate struct {
	version string
	report  Reporter
	own     http.Handler
	real    atomic.Pointer[http.Handler]
}

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

func (g *Gate) Open(
	h http.Handler,
) {
	if h == nil {
		panic("bootgate: Open requires a handler")
	}
	g.real.Store(&h)
}

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

func navigation(
	r *http.Request,
) bool {
	if r.Method != http.MethodGet || strings.HasPrefix(r.URL.Path, "/v0/") {
		return false
	}
	accept := r.Header.Get("Accept")
	htmlAt, jsonAt := strings.Index(accept, "text/html"), strings.Index(accept, "application/json")
	return htmlAt >= 0 && (jsonAt < 0 || htmlAt < jsonAt)
}

const page = `<!doctype html>
<html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width">
<meta name="color-scheme" content="light dark">
<title>Crowbar</title></head>
<body style="font:16px system-ui,sans-serif;max-width:32rem;margin:20vh auto;padding:0 16px">
<p>%s</p><p style="opacity:.6">%s</p><p>%s</p>
<script>setInterval(() => fetch("/v0/health").then((r) => r.json())
  .then((b) => { if (b.data && b.data.status === "ok") location.reload() }).catch(() => {}), 1000)</script>
</body></html>`

var pageWords = map[State][2]string{
	StateMigrating: {"Crowbar is updating your data… this page reloads by itself.", ""},
	StateHalted: {
		"Crowbar could not start.",
		"Fix the cause above and restart Crowbar. This page will reload once it is running.",
	},
}

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

const unencodable = `{"success":false,"error":"bootgate: the boot report cannot be encoded","code":"daemon_report_unencodable"}`

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
