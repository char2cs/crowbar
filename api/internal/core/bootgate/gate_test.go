package bootgate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/core/bootgate"
)

const (
	testVersion      = "1.2.3-test"
	healthPath       = "/v0/health"
	projectsPath     = "/v0/projects"
	chatsPath        = "/v0/chats"
	chatWSPath       = "/v0/chats/chat-1/ws"
	migratingMessage = "migrating storage (3/5)"
	haltedMessage    = "halted: ledger is corrupt"
)

type healthBody struct {
	Success bool       `json:"success"`
	Data    healthData `json:"data"`
}

// healthData.Status is asserted by its wire spelling: the desktop parses those
// exact strings, and reads one it does not know as ready.
type healthData struct {
	Status  string `json:"status"`
	Version string `json:"version"`
	PID     int    `json:"pid"`
	Detail  any    `json:"detail"`
}

type refusalBody struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
	Code    string `json:"code"`
}

func migrating() bootgate.Report {
	return bootgate.Report{
		State:   bootgate.StateMigrating,
		Message: migratingMessage,
		Detail:  map[string]any{"step": "backup"},
	}
}

func halted() bootgate.Report {
	return bootgate.Report{State: bootgate.StateHalted, Message: haltedMessage}
}

// bare is a gate middleware that adds nothing.
func bare(
	next http.Handler,
) http.Handler {
	return next
}

func newGate(
	report bootgate.Reporter,
) *bootgate.Gate {
	return bootgate.New(testVersion, report, bare)
}

func realAPI() http.Handler {
	return http.HandlerFunc(func(
		w http.ResponseWriter,
		_ *http.Request,
	) {
		w.Header().Set("X-Handled-By", "real")
		w.WriteHeader(http.StatusNoContent)
	})
}

func newRequest(
	method string,
	target string,
) *http.Request {
	return httptest.NewRequestWithContext(context.Background(), method, target, nil)
}

func send(
	h http.Handler,
	method string,
	target string,
) *httptest.ResponseRecorder {
	return serve(h, newRequest(method, target))
}

func serve(
	h http.Handler,
	req *http.Request,
) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func websocketUpgrade() *http.Request {
	req := newRequest(http.MethodGet, chatWSPath)
	req.Header.Set("Connection", "Upgrade")
	req.Header.Set("Upgrade", "websocket")
	return req
}

func decodeHealth(
	t *testing.T,
	rec *httptest.ResponseRecorder,
) healthBody {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body healthBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	return body
}

func assertRefused(
	t *testing.T,
	rec *httptest.ResponseRecorder,
	code string,
	message string,
) {
	t.Helper()
	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body refusalBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.Equal(t, code, body.Code)
	assert.Equal(t, message, body.Error)
}

func assertReal(
	t *testing.T,
	rec *httptest.ResponseRecorder,
) {
	t.Helper()
	assert.Equal(t, http.StatusNoContent, rec.Code)
	assert.Equal(t, "real", rec.Header().Get("X-Handled-By"))
}

func assertGateOrReal(
	t *testing.T,
	rec *httptest.ResponseRecorder,
) {
	t.Helper()
	if rec.Code == http.StatusServiceUnavailable {
		assertRefused(t, rec, "daemon_migrating", migratingMessage)
		return
	}
	assertReal(t, rec)
}

func serveWhenStarted(
	wg *sync.WaitGroup,
	start <-chan struct{},
	g *bootgate.Gate,
	rec *httptest.ResponseRecorder,
) {
	defer wg.Done()
	<-start
	g.ServeHTTP(rec, newRequest(http.MethodGet, projectsPath))
}

func openWhenStarted(
	wg *sync.WaitGroup,
	start <-chan struct{},
	g *bootgate.Gate,
) {
	defer wg.Done()
	<-start
	g.Open(realAPI())
}

func TestGate_ServesHealthBeforeOpen(
	t *testing.T,
) {
	g := newGate(migrating)

	body := decodeHealth(t, send(g, http.MethodGet, healthPath))

	assert.True(t, body.Success)
	assert.Equal(t, "migrating", body.Data.Status)
	assert.Equal(t, testVersion, body.Data.Version)
	assert.Equal(t, os.Getpid(), body.Data.PID)
	assert.Equal(t, map[string]any{"step": "backup"}, body.Data.Detail)
}

func TestGate_RefusesEverythingElseBeforeOpen(
	t *testing.T,
) {
	g := newGate(migrating)

	assertRefused(t, send(g, http.MethodGet, projectsPath), "daemon_migrating", migratingMessage)
	assertRefused(t, send(g, http.MethodPost, chatsPath), "daemon_migrating", migratingMessage)
	assertRefused(t, serve(g, websocketUpgrade()), "daemon_migrating", migratingMessage)
}

func TestGate_HaltedUsesItsOwnCode(
	t *testing.T,
) {
	g := newGate(halted)

	assert.Equal(t, "halted", decodeHealth(t, send(g, http.MethodGet, healthPath)).Data.Status)
	assertRefused(t, send(g, http.MethodGet, projectsPath), "daemon_halted", haltedMessage)
}

func TestGate_ReporterIsCalledPerRequest(
	t *testing.T,
) {
	progress := 1
	g := newGate(func() bootgate.Report {
		return bootgate.Report{State: bootgate.StateMigrating, Message: migratingMessage, Detail: fmt.Sprintf("step %d", progress)}
	})

	first := decodeHealth(t, send(g, http.MethodGet, healthPath))
	progress = 2
	second := decodeHealth(t, send(g, http.MethodGet, healthPath))

	assert.Equal(t, "step 1", first.Data.Detail)
	assert.Equal(t, "step 2", second.Data.Detail)
}

func TestGate_OpenHandsEveryRequestToTheRealHandler(
	t *testing.T,
) {
	g := newGate(migrating)
	g.Open(realAPI())

	assertReal(t, send(g, http.MethodGet, healthPath))
	assertReal(t, send(g, http.MethodGet, projectsPath))
}

func TestGate_OpenIsSafeUnderConcurrentRequests(
	t *testing.T,
) {
	const requests = 64
	g := newGate(migrating)
	start := make(chan struct{})
	var wg sync.WaitGroup
	recs := make([]*httptest.ResponseRecorder, requests)
	for i := range requests {
		recs[i] = httptest.NewRecorder()
		wg.Add(1)
		go serveWhenStarted(&wg, start, g, recs[i])
	}
	wg.Add(1)
	go openWhenStarted(&wg, start, g)
	close(start)
	wg.Wait()

	for _, rec := range recs {
		assertGateOrReal(t, rec)
	}
}

func TestGate_OpenRejectsNilHandler(
	t *testing.T,
) {
	g := newGate(migrating)

	assert.Panics(t, func() { g.Open(nil) })
}

func TestNew_RejectsNilReporter(
	t *testing.T,
) {
	assert.Panics(t, func() { bootgate.New(testVersion, nil, bare) })
}

func TestNew_RejectsNilMiddleware(
	t *testing.T,
) {
	assert.Panics(t, func() { bootgate.New(testVersion, migrating, nil) })
}

// An encode failure must not leave a 200 behind with half a body.
func TestGate_AnUnencodableReportAnswers500(
	t *testing.T,
) {
	g := newGate(func() bootgate.Report {
		return bootgate.Report{State: bootgate.StateMigrating, Message: migratingMessage, Detail: make(chan int)}
	})

	rec := send(g, http.MethodGet, healthPath)

	require.Equal(t, http.StatusInternalServerError, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	var body refusalBody
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.False(t, body.Success)
	assert.Equal(t, "daemon_report_unencodable", body.Code)
}

// The gate's own answers carry what the real API adds to its answers (CORS, so
// the webview can read them); once open, the real API adds its own.
func TestGate_AnswersThroughItsMiddlewareUntilOpen(
	t *testing.T,
) {
	through := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Through", "middleware")
			next.ServeHTTP(w, r)
		})
	}
	g := bootgate.New(testVersion, halted, through)

	health := send(g, http.MethodGet, healthPath)
	refused := send(g, http.MethodGet, projectsPath)
	preflight := send(g, http.MethodOptions, projectsPath)
	g.Open(realAPI())
	opened := send(g, http.MethodGet, projectsPath)

	assert.Equal(t, "halted", decodeHealth(t, health).Data.Status)
	assert.Equal(t, "middleware", health.Header().Get("X-Through"))
	assertRefused(t, refused, "daemon_halted", haltedMessage)
	assert.Equal(t, "middleware", refused.Header().Get("X-Through"))
	assert.Equal(t, "middleware", preflight.Header().Get("X-Through"))
	assertReal(t, opened)
	assert.Empty(t, opened.Header().Get("X-Through"))
}

// pinned fixes the two values that differ per run, so a body can be compared with
// its fixture byte for byte.
func pinned(
	body string,
) string {
	body = strings.Replace(body, fmt.Sprintf(`"pid":%d`, os.Getpid()), `"pid":4242`, 1)
	return strings.Replace(body, `"version":"`+testVersion+`"`, `"version":"0.1.0"`, 1)
}

// testdata/health-*.json are what the desktop shell's tests parse, so the states
// it must recognise are pinned here, in Go, where their spelling is decided.
func TestGate_HealthMatchesTheDesktopFixtures(
	t *testing.T,
) {
	type step struct {
		Number int    `json:"number"`
		Name   string `json:"name"`
	}
	type migratingDetail struct {
		Phase string `json:"phase"`
		Step  step   `json:"step"`
		Index int    `json:"index"`
		Total int    `json:"total"`
	}
	type haltedDetail struct {
		Reason  string `json:"reason"`
		Message string `json:"message"`
		Layout  int    `json:"layout"`
		Known   int    `json:"known"`
	}
	cases := map[string]bootgate.Report{
		"health-migrating.json": {
			State: bootgate.StateMigrating,
			Detail: migratingDetail{
				Phase: "pre-open",
				Step:  step{Number: 2, Name: "workspace-paths-table"},
				Index: 1,
				Total: 5,
			},
		},
		"health-halted.json": {
			State: bootgate.StateHalted,
			Detail: haltedDetail{
				Reason:  "layout_newer",
				Message: "this home is at layout 9 but this crowbar only knows layout 0: run a newer crowbar",
				Layout:  9,
				Known:   0,
			},
		},
	}
	for file, report := range cases {
		want, err := os.ReadFile(filepath.Join("testdata", file))
		require.NoError(t, err)

		rec := send(newGate(func() bootgate.Report { return report }), http.MethodGet, healthPath)

		assert.Equal(t, string(want), pinned(rec.Body.String()), file)
	}
}
