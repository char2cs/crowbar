package internal_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/pprof"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal"
	"github.com/char2cs/crowbar/api/internal/adapter/migrations"
	"github.com/char2cs/crowbar/api/internal/core/home"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/migrate"
	"github.com/char2cs/crowbar/api/internal/testutil"
	"github.com/char2cs/crowbar/api/internal/testutil/migratetest"
)

func TestNew_BootsFullStack(t *testing.T) {
	dir := t.TempDir()
	c, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(dir),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	assert.NotNil(t, c)
	assert.Equal(t, dir, c.Home())
	assert.Nil(t, c.Halt())
}

// A callback is handed the socket the daemon bound, so the daemon must say which
// one that is: the home as given, even when it is not clean, and none for TCP.
func TestNew_ReportsTheSocketItBound(t *testing.T) {
	t.Setenv(home.EnvVar, t.TempDir()+"/x/")
	h, err := home.Resolve()
	require.NoError(t, err)

	c, err := internal.New(context.Background(), "unix://", nil)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	assert.Equal(t, h.Socket(), c.Socket())
	assert.FileExists(t, c.Socket())
}

func TestNew_ReportsNoSocketOverTCP(t *testing.T) {
	c, err := internal.New(context.Background(), "tcp://127.0.0.1:0", nil, internal.WithHomeDir(t.TempDir()))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	assert.Empty(t, c.Socket())
}

func TestNew_InvalidHost_ReturnsError(t *testing.T) {
	_, err := internal.New(context.Background(), "bogus://x", nil, internal.WithHomeDir(t.TempDir()))
	assert.Error(t, err)
}

func TestNew_InvalidHomeDir_ReturnsError(t *testing.T) {
	file := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	_, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(filepath.Join(file, "home")),
	)
	assert.ErrorContains(t, err, "internal: migrate")
}

// The daemon holds its home while it runs, so a second one is refused before
// it opens a store, without keeping the socket it bound.
func TestNew_RefusesASecondDaemonOnTheSameHome(t *testing.T) {
	h := testutil.FixtureHome(t)
	_, err := boot(t.Context(), t, h)
	require.NoError(t, err)
	sock := newSocket(t)

	_, err = internal.New(t.Context(), "unix://"+sock, nil, internal.WithHomeDir(h.Root()))

	require.ErrorIs(t, err, migrate.ErrHomeLocked)
	assert.NoFileExists(t, sock)
}

func TestNew_RunsTheLegacyStepsBeforeServing(t *testing.T) {
	h := testutil.FixtureHome(t)

	d, err := boot(t.Context(), t, h)
	require.NoError(t, err)

	applied := migratetest.Applied(t, h)
	for step := 1; step <= 5; step++ {
		assert.Len(t, applied[step], 1, "step %d", step)
	}
	records := readLedger(t, h)
	assert.Equal(t, ledgerRecord{Kind: "run.end", Outcome: "completed"}, records[len(records)-1])
	health := d.get(t, "/v0/health")
	assert.Equal(t, http.StatusOK, health.status)
	assert.Equal(t, "ok", health.Data.Status, health.raw)
}

// A home newer than this binary is refused without a write: the daemon stays
// up, holding the home, and says why on the one route it serves.
func TestNew_HaltsOnANewerLayout(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "LAYOUT", "9\n")

	d, err := boot(t.Context(), t, h)
	require.NoError(t, err)

	health := d.get(t, "/v0/health")
	require.Equal(t, http.StatusOK, health.status)
	assert.Equal(t, "halted", health.Data.Status)
	halt := detail[migrate.Halt](t, health)
	assert.Equal(t, migrate.HaltLayoutNewer, halt.Reason)
	assert.Equal(t, *d.Halt(), halt)
	projects := d.get(t, "/v0/projects")
	assert.Equal(t, http.StatusServiceUnavailable, projects.status)
	assert.Equal(t, "daemon_halted", projects.Code)
	assert.Equal(t, halt.Message, projects.Error)
	_, err = hookserver.Read(h.Root())
	require.Error(t, err, "a halted daemon serves no hooks")
	assert.NoDirExists(t, h.MigrateDir())
	assert.Equal(t, []string{"LAYOUT 2", "state/.lock 0"}, testutil.TreeSnapshot(t, h))
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	assert.NoError(t, d.Run(cancelled))
}

// The webview reads the gate's answers cross-origin, as it reads the API's.
func TestNew_AHaltedDaemonAnswersTheWebview(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "LAYOUT", "9\n")
	d, err := boot(t.Context(), t, h)
	require.NoError(t, err)
	webview := http.Header{"Origin": {"tauri://localhost"}}

	health := call(t, d.client, http.MethodGet, "/v0/health", webview)
	refused := call(t, d.client, http.MethodPost, "/v0/projects", webview)
	preflight := call(t, d.client, http.MethodOptions, "/v0/projects",
		http.Header{"Origin": {"tauri://localhost"}, "Access-Control-Request-Method": {"POST"}})

	assert.Equal(t, http.StatusOK, health.status)
	assert.Equal(t, "tauri://localhost", health.header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusServiceUnavailable, refused.status)
	assert.Equal(t, "tauri://localhost", refused.header.Get("Access-Control-Allow-Origin"))
	assert.Equal(t, http.StatusNoContent, preflight.status)
	assert.Equal(t, "tauri://localhost", preflight.header.Get("Access-Control-Allow-Origin"))
	assert.Contains(t, preflight.header.Get("Access-Control-Allow-Methods"), "POST")
}

func TestNew_HaltsWhenAStepFails(t *testing.T) {
	h := testutil.FixtureHome(t)
	testutil.WriteFile(t, h, "state/view.db", "not a database")

	d, err := boot(t.Context(), t, h)
	require.NoError(t, err)

	health := d.get(t, "/v0/health")
	assert.Equal(t, "halted", health.Data.Status)
	halt := detail[migrate.Halt](t, health)
	assert.Equal(t, migrate.HaltStepFailed, halt.Reason)
	require.NotNil(t, halt.Step)
	assert.Equal(t, "workspace-paths-table", halt.Step.Name)
	sess, err := migrations.Begin(t.Context(), h, migrate.Options{})
	if sess != nil {
		require.NoError(t, sess.Close())
	}
	require.ErrorIs(t, err, migrate.ErrHomeLocked, "the halted daemon still holds the home")
}

func TestClose_ReleasesTheHomeLock(t *testing.T) {
	for name, layout := range map[string]string{"serving": "", "halted": "9\n"} {
		t.Run(name, func(t *testing.T) {
			h := testutil.FixtureHome(t)
			if layout != "" {
				testutil.WriteFile(t, h, "LAYOUT", layout)
			}
			c, err := internal.New(t.Context(), "unix://"+newSocket(t), nil, internal.WithHomeDir(h.Root()))
			require.NoError(t, err)

			c.Close()

			again, err := internal.New(t.Context(), "unix://"+newSocket(t), nil, internal.WithHomeDir(h.Root()))
			require.NoError(t, err)
			again.Close()
		})
	}
}

// An interrupted boot is not a halt: the run ends interrupted, the home is let
// go, and the next boot resumes.
func TestNew_CancelledDuringMigrationReturnsTheContextError(t *testing.T) {
	h := testutil.FixtureHome(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	sock := newSocket(t)

	c, err := internal.New(ctx, "unix://"+sock, nil, internal.WithHomeDir(h.Root()))

	require.ErrorIs(t, err, context.Canceled)
	assert.Nil(t, c)
	records := readLedger(t, h)
	assert.Equal(t, ledgerRecord{Kind: "run.end", Outcome: "interrupted"}, records[len(records)-1])
	assert.NoFileExists(t, sock)
	requireHomeFree(t, h)
}

// A boot cancelled once the stores are open tears down everything it started,
// all the way, though its context is done; every goroutine New starts inherits
// its pprof label, so none may outlive it.
func TestNew_CancelledDuringAPostOpenStepStopsWhatItStarted(t *testing.T) {
	h := testutil.FixtureHome(t)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	cancelPostOpen := slog.New(onStepStart(func(phase string) {
		if phase == "post-open" {
			cancel()
		}
	}))
	var err error

	pprof.Do(ctx, pprof.Labels("internal_new", t.Name()), func(ctx context.Context) {
		_, err = internal.New(ctx, "unix://"+newSocket(t), nil,
			internal.WithHomeDir(h.Root()), internal.WithMigrateLog(cancelPostOpen))
	})

	require.ErrorIs(t, err, context.Canceled)
	assert.NotContains(t, err.Error(), "\n", "the teardown failed nothing")
	records := readLedger(t, h)
	assert.Equal(t, ledgerRecord{Kind: "run.end", Outcome: "interrupted"}, records[len(records)-1])
	requireHomeFree(t, h)
	assert.Eventually(t, func() bool { return !labelled(t, "internal_new", t.Name()) },
		5*time.Second, 10*time.Millisecond, "goroutines New started outlived it")
}

func TestNew_DefaultDirs_UsesHome(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv(home.EnvVar, "")
	c, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)
	assert.NotNil(t, c)
}

func TestRun_CancelledContext_Returns(t *testing.T) {
	c, err := internal.New(
		context.Background(),
		"tcp://127.0.0.1:0",
		nil,
		internal.WithHomeDir(t.TempDir()),
	)
	require.NoError(t, err)
	t.Cleanup(c.Close)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = c.Run(ctx)
	assert.NoError(t, err)
}

// A vendor CLI's HTTP hook reaches the real router over loopback TCP, and nothing
// else of the API is served on that port.
func TestRun_ServesOnlyTheRunnerHookRouteOnALoopbackPort(t *testing.T) {
	home := t.TempDir()
	c, err := internal.New(context.Background(), "tcp://127.0.0.1:0", nil, internal.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(c.Close)
	url, err := hookserver.Read(home)
	require.NoError(t, err, "the daemon publishes where hooks are served")
	require.True(t, strings.HasPrefix(url, "http://127.0.0.1:"), url)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		<-done
	})

	fetch := func(method, target string, payload io.Reader) string {
		req, err := http.NewRequestWithContext(ctx, method, target, payload)
		require.NoError(t, err)
		resp, err := http.DefaultClient.Do(req)
		require.NoError(t, err)
		defer func() { _ = resp.Body.Close() }()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return string(b)
	}
	assert.Contains(t, fetch(http.MethodPost, url+"/v0/projects/p/repos/r/chats/runners/seg/hooks/claude/tool_pre", strings.NewReader("{}")),
		"repository not found", "the real router, not the gate, answered")
	assert.Equal(t, "404 page not found\n", fetch(http.MethodGet, url+"/v0/projects", nil),
		"the gate refuses the rest of the API")
}

// progress is the health detail of a daemon still migrating.
type progress struct {
	Phase string     `json:"phase"`
	Step  migrate.ID `json:"step"`
	Index int        `json:"index"`
	Total int        `json:"total"`
}

// While a step runs, health names it, from the first pre-open step on, and
// every other request is refused with the same words.
func TestReport_NamesTheStepThatIsRunning(t *testing.T) {
	h := testutil.FixtureHome(t)
	sock := newSocket(t)
	client := unixClient(t, sock)
	var healths, refusals []answer
	askAtEachStep := slog.New(onStepStart(func(string) {
		healths = append(healths, call(t, client, http.MethodGet, "/v0/health", nil))
		refusals = append(refusals, call(t, client, http.MethodGet, "/v0/projects", nil))
	}))

	c, err := internal.New(t.Context(), "unix://"+sock, nil,
		internal.WithHomeDir(h.Root()), internal.WithMigrateLog(askAtEachStep))
	require.NoError(t, err)
	t.Cleanup(c.Close)

	require.Len(t, healths, 5)
	for _, health := range healths {
		assert.Equal(t, http.StatusOK, health.status)
		assert.Equal(t, "migrating", health.Data.Status, health.raw)
	}
	assert.Equal(t, progress{Phase: "pre-open", Step: migrate.ID{Number: 2, Name: "workspace-paths-table"}, Index: 1, Total: 5},
		detail[progress](t, healths[0]))
	assert.Equal(t, progress{Phase: "post-open", Step: migrate.ID{Number: 1, Name: "project-homes"}, Index: 2, Total: 5},
		detail[progress](t, healths[1]))
	assert.Equal(t, http.StatusServiceUnavailable, refusals[0].status)
	assert.Equal(t, "daemon_migrating", refusals[0].Code)
	assert.Equal(t, "Crowbar is upgrading its data (step 1 of 5: workspace-paths-table)", refusals[0].Error)
	assert.Equal(t, "Crowbar is upgrading its data (step 5 of 5: hook-deliveries)", refusals[4].Error)
}
