//go:build integration && unix

// Package scripted_test proves the agent session lifecycle end to end against
// both built-in descriptors, with the vendor CLIs replaced by a scripted
// emulator of their real wire (fake_*_test.go): the shipped claude.yaml and
// codex.yaml, a real daemon, real PTYs, the real `crowbar hook` relay over the
// daemon's unix socket and a real app-server websocket. No credentials and no
// network — every healthy and faulty behaviour is declared per test.
package scripted_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// askTimeoutSeconds shortens both descriptors' answer budget so an
// unanswered ask expires within a test.
const askTimeoutSeconds = 2

func TestMain(m *testing.M) {
	if os.Getenv(envScript) != "" {
		switch filepath.Base(os.Args[0]) {
		case "claude":
			os.Exit(runClaude(os.Args[1:]))
		case "codex":
			os.Exit(runCodex(os.Args[1:]))
		}
	}
	gin.SetMode(gin.TestMode)
	level := slog.LevelError
	if os.Getenv("SCRIPTED_DEBUG") != "" {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))
	dir, err := os.MkdirTemp("", "scripted-hook-")
	if err != nil {
		panic(err)
	}
	bin := filepath.Join(dir, "crowbar")
	build := exec.Command("go", "build", "-tags", "noEmbed", "-o", bin, "./cmd/crowbar")
	build.Dir = apiRoot()
	if out, err := build.CombinedOutput(); err != nil {
		panic(fmt.Sprintf("build crowbar: %v\n%s", err, out))
	}
	_ = os.Setenv("CROWBAR_HOOK_BIN", bin)
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func apiRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// daemon is one boot of the backend, served on the unix socket the hook relay
// dials. A restart is a second boot over the same home.
type daemon struct {
	app      *app.Container
	eng      *engine.Container
	adapters *adapter.Container
	ln       net.Listener
}

func bootDaemon(t *testing.T, home string) *daemon {
	t.Helper()
	ctx := context.Background()
	eng, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)
	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	container, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)
	router := gin.New()
	v0.New(container, eng).Register(router.Group("/v0"))
	ln, err := transports.NewSocket("unix://")
	require.NoError(t, err)
	go func() { _ = http.Serve(ln, router) }() //nolint:gosec // test server on a private unix socket
	return &daemon{app: container, eng: eng, adapters: adapters, ln: ln}
}

// stop ends the daemon the way a crash does: no graceful drain, and every
// CLI it spawned dies with it.
func (d *daemon) stop() {
	_ = d.ln.Close()
	d.app.Close()
	d.eng.Close()
	_ = d.adapters.Close()
}

// rig is one test's daemon, workspace, fake CLIs and their script and log.
type rig struct {
	t      *testing.T
	home   string
	d      *daemon
	wsID   string
	script string
	log    string
}

func newRig(t *testing.T) *rig {
	t.Helper()
	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)
	kit.IsolateProviderHomes(t)
	scratch := t.TempDir()
	r := &rig{t: t, home: home, script: filepath.Join(scratch, "script.yaml"), log: filepath.Join(scratch, "cli.log")}
	t.Setenv(envScript, r.script)
	t.Setenv(envLog, r.log)
	r.setScript("turn: []\n")
	installFakes(t, home, filepath.Join(scratch, "bin"))
	r.d = bootDaemon(t, home)
	t.Cleanup(func() { r.d.stop() })
	repo := kit.InitRepo(t)
	r.wsID = r.workspace(repo)
	return r
}

// installFakes links the test binary in as `claude` and `codex` and points
// the SHIPPED descriptors at them — only the binary and the ask budget change.
func installFakes(t *testing.T, home, bin string) {
	t.Helper()
	self, err := os.Executable()
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(bin, 0o750))
	require.NoError(t, os.MkdirAll(filepath.Join(home, "descriptors"), 0o750))
	shipped := filepath.Join(apiRoot(), "internal", "engine", "agents", "internal", "protocol", "internal",
		"descriptor", "descriptors-v3")
	for _, id := range []string{"claude", "codex"} {
		require.NoError(t, os.Symlink(self, filepath.Join(bin, id)))
		raw, err := os.ReadFile(filepath.Join(shipped, id+".yaml"))
		require.NoError(t, err)
		doc := regexp.MustCompile(`(?m)^(\s*)cmd: `+id+`$`).ReplaceAllString(string(raw), "${1}cmd: "+filepath.Join(bin, id))
		doc = strings.ReplaceAll(doc, "["+id+", ", "["+filepath.Join(bin, id)+", ")
		doc = strings.ReplaceAll(doc, "timeout_seconds: 270", fmt.Sprintf("timeout_seconds: %d", askTimeoutSeconds))
		require.NotContains(t, doc, "cmd: "+id+"\n", "every launch must reach the fake")
		require.NoError(t, os.WriteFile(filepath.Join(home, "descriptors", id+".yaml"), []byte(doc), 0o600))
	}
}

func (r *rig) workspace(repoPath string) string {
	ctx := context.Background()
	u := r.d.app.Usecases
	project, err := u.ProjectImport.Create(ctx, "scripted", repoPath)
	require.NoError(r.t, err)
	repo, err := u.ProjectImport.ImportRepo(ctx, project.ID, "", repoPath)
	require.NoError(r.t, err)
	ws, err := u.Workspace.CreateChild(ctx, workspace.CreateChildInput{
		RepoID: repo.ID, ProjectID: project.ID, RepoPath: repo.Path, RemoteURL: repo.RemoteURL,
		Branch: "scripted", ParentBranch: repo.DefaultBranch,
	})
	require.NoError(r.t, err)
	return ws.ID
}

// restart kills the daemon mid-flight and boots it again over the same home.
func (r *rig) restart() {
	r.t.Helper()
	r.d.stop()
	r.d = bootDaemon(r.t, r.home)
}

func (r *rig) setScript(yaml string) {
	r.t.Helper()
	require.NoError(r.t, os.WriteFile(r.script, []byte(yaml), 0o600))
}

func (r *rig) runners() agentusecase.RunnerUsecase { return r.d.app.Usecases.AgentRunner }

func (r *rig) spawn(provider string) string {
	r.t.Helper()
	chatID, _, err := r.runners().SpawnChat(context.Background(), r.wsID, provider)
	require.NoError(r.t, err)
	return chatID
}

func (r *rig) send(chatID, text string) error {
	_, err := r.runners().SubmitPrompt(context.Background(), chatID, text, uuid.NewString(), "", nil)
	return err
}

func (r *rig) snapshot(chatID string) agentusecase.ChatSnapshot {
	r.t.Helper()
	snap, err := r.d.app.Usecases.AgentChat.ChatSnapshot(context.Background(), chatID)
	require.NoError(r.t, err)
	return snap
}

func (r *rig) live(chatID string) (bool, string) {
	runner, err := r.runners().LiveRunnerForChat(context.Background(), chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return false, ""
	}
	require.NoError(r.t, err)
	return true, runner.ID
}

// said reports the ledger's user/assistant texts in order.
func (r *rig) said(chatID string) []string {
	r.t.Helper()
	page, err := r.d.app.Usecases.AgentChat.ReadMessages(context.Background(), chatID, 0, 0, 0)
	require.NoError(r.t, err)
	out := make([]string, 0, len(page.Items))
	for _, m := range page.Items {
		out = append(out, m.Role+": "+m.Text)
	}
	return out
}

func (r *rig) working(chatID string) bool {
	return r.snapshot(chatID).Chat.Working
}

func (r *rig) choices(chatID string) []domain.ActivityChoice {
	r.t.Helper()
	pending, err := r.d.app.Usecases.AgentTurn.ReadPendingChoices(context.Background(), chatID)
	require.NoError(r.t, err)
	return pending
}

// eventually waits for cond, polling what the test can observe.
func (r *rig) eventually(cond func() bool, msg string) {
	r.t.Helper()
	require.Eventually(r.t, cond, 20*time.Second, 20*time.Millisecond, "%s\nCLI log:\n%s", msg, logTail{r})
}

// logTail renders the CLI log lazily, only when a wait fails.
type logTail struct{ r *rig }

func (l logTail) String() string { return l.r.dump() }

type logEntry map[string]any

// logged is every fact the fake CLIs recorded of kind, in order.
func (r *rig) logged(kind string) []logEntry {
	f, err := os.Open(r.log)
	if err != nil {
		return nil
	}
	defer func() { _ = f.Close() }()
	var out []logEntry
	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 1<<20), 1<<24)
	for scan.Scan() {
		var e logEntry
		if json.Unmarshal(scan.Bytes(), &e) == nil && e["kind"] == kind {
			out = append(out, e)
		}
	}
	return out
}

// prompted reports whether a fake CLI received a prompt containing text.
func (r *rig) prompted(text string) bool {
	for _, e := range r.logged("prompt") {
		if s, _ := e["text"].(string); strings.Contains(s, text) {
			return true
		}
	}
	return false
}

// dump is everything the fake CLIs logged, for a failure message.
func (r *rig) dump() string {
	f, err := os.Open(r.log)
	if err != nil {
		return ""
	}
	defer func() { _ = f.Close() }()
	b, _ := io.ReadAll(f)
	return string(b)
}
