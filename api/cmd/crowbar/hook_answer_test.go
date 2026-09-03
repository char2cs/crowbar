package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeDaemon struct {
	mu    sync.Mutex
	paths []string

	ack string

	held chan struct{}

	answer string

	awaiting chan struct{}

	awaitOnce sync.Once
}

func newFakeDaemon(t *testing.T) (*fakeDaemon, string) {
	t.Helper()
	sock := filepath.Join(shortSocketDir(t), "d.sock")
	ln, err := net.Listen("unix", sock)
	require.NoError(t, err)

	d := &fakeDaemon{ack: `{"success":true}`, awaiting: make(chan struct{})}
	srv := &http.Server{Handler: http.HandlerFunc(d.serve)}
	go func() { _ = srv.Serve(ln) }()
	t.Cleanup(func() {
		_ = srv.Close()
		_ = ln.Close()
	})
	return d, "unix://" + sock
}

func (d *fakeDaemon) serve(w http.ResponseWriter, r *http.Request) {
	d.mu.Lock()
	d.paths = append(d.paths, r.URL.Path)
	ack, answer, held := d.ack, d.answer, d.held
	d.mu.Unlock()

	switch {
	case hasSuffix(r.URL.Path, "/hooks/await"):
		d.awaitOnce.Do(func() { close(d.awaiting) })
		if held != nil {
			select {
			case <-held:
			case <-r.Context().Done():
				return
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(answer))
	case hasSuffix(r.URL.Path, "/hooks/abandon"):
		w.WriteHeader(http.StatusAccepted)
	default:
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(ack))
	}
}

func hasSuffix(path, suffix string) bool {
	return len(path) >= len(suffix) && path[len(path)-len(suffix):] == suffix
}

func (d *fakeDaemon) reached(path string) bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	for _, p := range d.paths {
		if hasSuffix(p, path) {
			return true
		}
	}
	return false
}

func awaitAck(waitMS int64) string {
	body, _ := json.Marshal(map[string]any{
		"success": true,
		"data":    map[string]any{"await": map[string]any{"choiceId": "c1", "waitMs": waitMS}},
	})
	return string(body)
}

func relay(host string, out *bytes.Buffer) hookRun {
	return hookRun{
		Event: "permission", Segment: "seg-1", Provider: "claude",
		Project: "p", Repo: "r", Workspace: "w",
		Payload: []byte(`{"tool_name":"Bash"}`), Host: host, Out: out,
	}
}

func TestRegression_HookWithNoDaemonPrintsNothingAndNeverWaits(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	dead := "unix://" + filepath.Join(shortSocketDir(t), "nobody.sock")

	var out bytes.Buffer
	err := runHook(relay(dead, &out))

	require.Error(t, err, "the undelivered hook is reported on stderr, never stdout")
	assert.Empty(t, out.String(), "a hook that could not deliver must print NOTHING")
}

func TestRegression_AnAcknowledgementWithNoDirectiveNeverReachesTheWaitEndpoint(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	daemon, host := newFakeDaemon(t)

	var out bytes.Buffer
	require.NoError(t, runHook(relay(host, &out)))

	assert.Empty(t, out.String())
	assert.False(t, daemon.reached("/hooks/await"),
		"a relay with nothing to wait for must not open a long-poll")
}

func TestRunHook_PrintsTheDaemonsVerdictVerbatim(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	daemon, host := newFakeDaemon(t)
	verdict := `{"hookSpecificOutput":{"hookEventName":"PermissionRequest",` +
		`"decision":{"behavior":"allow"}}}`
	daemon.mu.Lock()
	daemon.ack = awaitAck(30_000)
	daemon.answer = `{"success":true,"data":{"stdout":` + mustQuote(verdict) + `}}`
	daemon.mu.Unlock()

	var out bytes.Buffer
	require.NoError(t, runHook(relay(host, &out)))

	assert.Equal(t, verdict+"\n", out.String())
	assert.True(t, daemon.reached("/hooks/await"))
}

func TestRunHook_APromptResolvedElsewherePrintsNothing(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	daemon, host := newFakeDaemon(t)
	daemon.mu.Lock()
	daemon.ack = awaitAck(30_000)
	daemon.answer = `{"success":true,"data":{"stdout":""}}`
	daemon.mu.Unlock()

	var out bytes.Buffer
	require.NoError(t, runHook(relay(host, &out)))

	assert.Empty(t, out.String())
}

func TestRegression_AnEmptyAwaitBodyIsNoDecisionRatherThanAnError(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	daemon, host := newFakeDaemon(t)
	daemon.mu.Lock()
	daemon.ack = awaitAck(30_000)
	daemon.answer = ""
	daemon.mu.Unlock()

	var out bytes.Buffer
	require.NoError(t, runHook(relay(host, &out)),
		"an empty 2xx body must be treated as no decision, not as a decode failure")
	assert.Empty(t, out.String(), "no decision means nothing is printed")
}

func TestRegression_ASignalledRelayReportsAndExitsWithoutPrinting(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	daemon, host := newFakeDaemon(t)
	daemon.mu.Lock()
	daemon.held = make(chan struct{})
	daemon.answer = `{"success":true,"data":{"stdout":"NEVER"}}`
	daemon.mu.Unlock()

	signals := make(chan os.Signal, 1)
	envelope := hookEnvelope{
		DeliveryID: "delivery-1", Project: "p", Repo: "r", Workspace: "w",
	}
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- awaitHookAnswerOn(envelope, host, 30_000, &out, signals) }()

	<-daemon.awaiting
	signals <- syscall.SIGTERM

	require.NoError(t, <-done, "being killed is not a failure the CLI should see")
	assert.Empty(t, out.String(), "a relay that was killed must print NOTHING")
	assert.True(t, daemon.reached("/hooks/abandon"),
		"the chat must be told this prompt was decided elsewhere")
}

func TestAwaitHookAnswer_ADaemonThatVanishesMidWaitPrintsNothing(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	dead := "unix://" + filepath.Join(shortSocketDir(t), "gone.sock")

	var out bytes.Buffer
	err := awaitHookAnswerOn(hookEnvelope{DeliveryID: "d1"}, dead, 5_000, &out, nil)

	require.Error(t, err)
	assert.Empty(t, out.String())
}

func TestAwaitHookAnswer_ARefusedWaitIsNotADecision(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	daemon, host := newFakeDaemon(t)

	for _, body := range []string{"not json", `{"success":true,"data":{}}`} {
		daemon.mu.Lock()
		daemon.answer = body
		daemon.mu.Unlock()

		var out bytes.Buffer
		_ = awaitHookAnswerOn(hookEnvelope{DeliveryID: "d1"}, host, 5_000, &out, nil)
		assert.Empty(t, out.String())
	}
}

func TestAwaitDirective_OnlyAPositiveBudgetHoldsTheRelay(t *testing.T) {
	testCases := []struct {
		name string
		body string
		want bool
	}{
		{name: "empty body", body: ""},
		{name: "undecodable", body: "{{{"},
		{name: "plain 202", body: `{"success":true}`},
		{name: "no await", body: `{"success":true,"data":{}}`},
		{name: "zero budget", body: awaitAck(0)},
		{name: "negative budget", body: awaitAck(-1)},
		{name: "a real directive", body: awaitAck(1_000), want: true},
	}
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, waiting := awaitDirective([]byte(tc.body))
			assert.Equal(t, tc.want, waiting)
		})
	}
}

func TestAwaitHookAnswer_AZeroBudgetReturnsWithoutCallingTheDaemon(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	daemon, host := newFakeDaemon(t)

	var out bytes.Buffer
	require.NoError(t, awaitHookAnswerOn(hookEnvelope{DeliveryID: "d1"}, host, 0, &out, nil))

	assert.False(t, daemon.reached("/hooks/await"))
	assert.Empty(t, out.String())
}

func TestAwaitHookAnswer_TrapsTheProcessSignal(t *testing.T) {
	t.Setenv("CROWBAR_HOME", t.TempDir())
	daemon, host := newFakeDaemon(t)
	daemon.mu.Lock()
	daemon.held = make(chan struct{})
	daemon.mu.Unlock()

	envelope := hookEnvelope{DeliveryID: "d1", Project: "p", Repo: "r", Workspace: "w"}
	var out bytes.Buffer
	done := make(chan error, 1)
	go func() { done <- awaitHookAnswer(envelope, host, 30_000, &out) }()

	<-daemon.awaiting
	require.NoError(t, syscall.Kill(syscall.Getpid(), syscall.SIGTERM))

	require.NoError(t, <-done)
	assert.Empty(t, out.String())
	assert.True(t, daemon.reached("/hooks/abandon"))
}

func mustQuote(s string) string {
	quoted, err := json.Marshal(s)
	if err != nil {
		return `""`
	}
	return string(quoted)
}
