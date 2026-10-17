package bootgate_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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
		State:   "migrating",
		Message: migratingMessage,
		Detail:  map[string]any{"step": "backup"},
	}
}

func halted() bootgate.Report {
	return bootgate.Report{State: "halted", Message: haltedMessage}
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
	g := bootgate.New(testVersion, migrating)

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
	g := bootgate.New(testVersion, migrating)

	assertRefused(t, send(g, http.MethodGet, projectsPath), "daemon_migrating", migratingMessage)
	assertRefused(t, send(g, http.MethodPost, chatsPath), "daemon_migrating", migratingMessage)
	assertRefused(t, serve(g, websocketUpgrade()), "daemon_migrating", migratingMessage)
}

func TestGate_HaltedUsesItsOwnCode(
	t *testing.T,
) {
	g := bootgate.New(testVersion, halted)

	assert.Equal(t, "halted", decodeHealth(t, send(g, http.MethodGet, healthPath)).Data.Status)
	assertRefused(t, send(g, http.MethodGet, projectsPath), "daemon_halted", haltedMessage)
}

func TestGate_ReporterIsCalledPerRequest(
	t *testing.T,
) {
	progress := 1
	g := bootgate.New(testVersion, func() bootgate.Report {
		return bootgate.Report{State: "migrating", Message: migratingMessage, Detail: fmt.Sprintf("step %d", progress)}
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
	g := bootgate.New(testVersion, migrating)
	g.Open(realAPI())

	assertReal(t, send(g, http.MethodGet, healthPath))
	assertReal(t, send(g, http.MethodGet, projectsPath))
}

func TestGate_OpenIsSafeUnderConcurrentRequests(
	t *testing.T,
) {
	const requests = 64
	g := bootgate.New(testVersion, migrating)
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
	g := bootgate.New(testVersion, migrating)

	assert.Panics(t, func() { g.Open(nil) })
}

func TestNew_RejectsNilReporter(
	t *testing.T,
) {
	assert.Panics(t, func() { bootgate.New(testVersion, nil) })
}
