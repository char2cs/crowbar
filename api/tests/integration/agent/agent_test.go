//go:build integration

// Package agent_test is the acceptance test for the agentic engine (Task 19):
// it drives the REAL claude/codex CLIs through Crowbar's Go stack end to end.
//
// The one thing kit.BuildEnv cannot exercise is the hook round trip: claude's
// SessionStart hook shells out to `crowbar hook session_start`, which POSTs to
// the daemon over a UNIX SOCKET at transports.SocketPath("unix://") — but
// kit.BuildEnv serves the daemon over an httptest/TCP server, which that
// subprocess can never reach. So this package builds its own from-scratch
// daemon (engine -> adapter -> app -> v0 router) and serves it on the real
// unix socket, wires a real built `crowbar` binary as the hook seam
// (CROWBAR_HOOK_BIN), and drives the agent usecase directly (SpawnChat /
// SwitchProvider) rather than through HTTP — the point of the test is proving
// the hook comes BACK over the socket, not re-testing the REST surface.
package agent_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain silences logs and enables gin test mode, mirroring every other
// integration package.
func TestMain(m *testing.M) {
	kit.Main(m)
}

// harness is a from-scratch daemon wired the same way cmd/crowbar's `serve`
// wires it, but pinned to a temp CROWBAR_HOME and served on the real unix
// socket instead of a fixed system path.
type harness struct {
	home string
	app  *app.Container
	eng  *engine.Container
}

// newHarness builds a real crowbar binary, points CROWBAR_HOME + the
// CROWBAR_HOOK_BIN test seam at a fresh temp home, and serves the real v0
// router on the same unix socket a spawned claude/codex's `crowbar hook`
// subprocess will dial (it inherits CROWBAR_HOME via os.Environ()).
func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)

	bin := buildCrowbarBinary(t)
	t.Setenv("CROWBAR_HOOK_BIN", bin)

	eng, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	appContainer, err := app.New(ctx, eng, adapters)
	require.NoError(t, err)

	router := gin.New()
	apiContainer := v0.New(appContainer, eng)
	apiContainer.Register(router.Group("/v0"))

	ln, err := transports.NewSocket("unix://")
	require.NoError(t, err, "bind daemon unix socket (must be the same path `crowbar hook` resolves via CROWBAR_HOME)")
	go func() { _ = http.Serve(ln, router) }()

	// Registered LAST so it runs FIRST (t.Cleanup is LIFO): stop accepting hook
	// traffic and tear down live realtime goroutines before the adapter's
	// SQLite handles (registered above, so it closes after) and t.TempDir's
	// RemoveAll run. Callers that spawn a vendor CLI must additionally
	// t.Cleanup a Terminal.Kill of its session BEFORE calling newHarness's
	// caller returns, so that cleanup (registered later still) runs before
	// even this one and the CLI process is dead before its cwd's temp tree is
	// removed (the terminal suite's documented worktree-busy flake).
	t.Cleanup(func() {
		_ = ln.Close()
		appContainer.Close()
	})

	return &harness{home: home, app: appContainer, eng: eng}
}

// buildCrowbarBinary compiles a real ./cmd/crowbar binary so the daemon's
// spawned claude/codex CLI has something to shell out to on SessionStart/Stop.
// kit.BuildEnv's httptest server never needed this binary because it never
// exercises the hook path; this is the one integration surface that does.
func buildCrowbarBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "crowbar")
	cmd := exec.Command("go", "build", "-tags", "noEmbed", "-o", bin, "./cmd/crowbar")
	cmd.Dir = apiRootDir(t)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build crowbar binary: %s", out)
	return bin
}

// apiRootDir resolves the api/ module root from this test file's own path
// (tests/integration/agent/agent_test.go -> api/), independent of whatever
// directory `go test` happens to run from.
func apiRootDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve test file path")
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

// importRepoAndWorkspace creates a project + repo and a REAL Crowbar-managed
// CHILD workspace (a genuine `git worktree`, not the adopted repo-home stub)
// by calling the app usecases directly — no HTTP/WS round trip needed since
// those endpoints are async-broadcast-only in production and we already hold
// the app container.
//
// The child-workspace requirement is load-bearing, not stylistic:
// agent.WorkspaceReader (internal/app/usecases/container.go's
// agentWorkspaceReader) derives SpawnChat's cwd from the workspace row's stored
// WorktreePath (WorktreeDir returns it verbatim). A managed child worktree's
// stored WorktreePath is a real on-disk directory under crowbar home (worktree.
// CreateChild creates it there), so both the cwd and the sibling chats dir
// resolve to real, reap-able paths. The adopted repo-home workspace's stored
// WorktreePath is instead the external repoPath (the user's real checkout,
// OUTSIDE home) — spawning there is still valid (its chats reroot under home via
// AgentChatsDir, Task 7), but this harness deliberately uses a managed child so
// the assertions observe the simple sibling-of-worktree layout.
func (h *harness) importRepoAndWorkspace(
	t *testing.T,
	name string,
	repoPath string,
) (projectID, repoID, wsID string) {
	t.Helper()
	ctx := context.Background()

	project, err := h.app.Usecases.ProjectImport.Create(ctx, name, repoPath)
	require.NoError(t, err, "create project")

	repo, err := h.app.Usecases.ProjectImport.ImportRepo(ctx, project.ID, repoPath)
	require.NoError(t, err, "import repo")

	ws, err := h.app.Usecases.Worktree.CreateChild(ctx, worktree.CreateChildInput{
		RepoID:       repo.ID,
		ProjectID:    project.ID,
		RepoPath:     repo.Path,
		RemoteURL:    repo.RemoteURL,
		Branch:       "agent-test-branch",
		ParentBranch: repo.DefaultBranch,
	})
	require.NoError(t, err, "create managed child workspace")
	require.NotEmpty(t, ws.WorktreePath, "managed workspace must have a real worktree dir")

	return project.ID, repo.ID, ws.ID
}

// requireCLI skips the test when name is not runnable from this process.
// exec.LookPath is tried first (the PATH the go test process actually has);
// ~/.local/bin/<name> is this machine's documented fallback location.
func requireCLI(t *testing.T, name string) {
	t.Helper()
	if _, err := exec.LookPath(name); err == nil {
		return
	}
	fallback := filepath.Join(os.Getenv("HOME"), ".local", "bin", name)
	if _, err := os.Stat(fallback); err == nil {
		return
	}
	t.Skipf("%s not installed", name)
}

// liveRunnerTerminalSession returns the PTY of the runner currently placed on
// chatID, failing the test if the chat is dormant (no live runner — which, for a
// chat whose CLI was just spawned, means the CLI died on startup).
//
// It replaces the old segmentTerminalSessionID: a chat no longer carries segment
// rows at all, and the PTY belongs to the RUNNER — the process — which is joined on
// at read time.
func liveRunnerTerminalSession(t *testing.T, h *harness, chatID string) string {
	t.Helper()
	runner, err := h.app.Usecases.Agent.LiveRunnerForChat(context.Background(), chatID)
	require.NoError(t, err, "chat %s has no live runner (its CLI never started, or died immediately)", chatID)
	require.NotEmpty(t, runner.TerminalSession)
	return runner.TerminalSession
}

// runnerTerminalSession returns runnerID's PTY.
//
// It needs NO polling, and that is a real consequence of the refactor rather than a
// stylistic choice. The old waitForSegmentTerminalSessionID had to poll the chat's
// segment list after a switch, because the segment rows were written with an async
// ax.Send and the read model could trail the id SwitchProvider had already returned.
// Runner commands are SendWait: when SwitchProvider returns a runner id, that runner
// is durable and visible. So this is a plain read.
func runnerTerminalSession(t *testing.T, h *harness, runnerID string) string {
	t.Helper()
	runner, err := h.app.Repositories.AgentRunner.Get(context.Background(), runnerID)
	require.NoError(t, err, "runner %s is not live", runnerID)
	require.NotEmpty(t, runner.TerminalSession)
	return runner.TerminalSession
}

// liveRunnerID returns the id of the runner placed on chatID, or "" if the chat is
// dormant. It is the runner-era answer to "what is this chat's active segment?" —
// except that a dormant chat is a real answer here, not a broken one: the live row
// exists exactly while the vendor CLI's PTY does.
func liveRunnerID(t *testing.T, h *harness, chatID string) string {
	t.Helper()
	runner, err := h.app.Usecases.Agent.LiveRunnerForChat(context.Background(), chatID)
	if errors.Is(err, agentrunner.ErrNotFound) {
		return ""
	}
	require.NoError(t, err)
	return runner.ID
}

// chatSessionIDs lists the conversations chatID has hosted, oldest first — the
// append-only history that succeeded the chat's embedded segment rows.
func chatSessionIDs(t *testing.T, h *harness, chatID string) []string {
	t.Helper()
	convs, err := h.app.Usecases.Agent.ConversationsForChat(context.Background(), chatID)
	require.NoError(t, err)
	out := make([]string, 0, len(convs))
	for _, c := range convs {
		out = append(out, c.SessionID)
	}
	return out
}

// nudgeUntil polls check every 250ms for up to timeout, periodically writing a
// bare Enter into the PTY session. This is test-harness driving, not product
// behaviour: it dismisses claude's "do you trust the files in this folder?"
// first-run prompt, which the Phase-0 spike
// (docs/superpowers/specs/spike-2026-07-05-agentic/drive.py) found blocks
// SessionStart until dismissed. A stray Enter is a safe no-op once the prompt
// is already gone (submitting an empty input box does nothing in the Claude
// Code TUI). Returns the last check result (possibly the zero value) so the
// caller can assert with a rich failure message rather than failing here.
func nudgeUntil[T any](
	h *harness,
	termSessID string,
	timeout time.Duration,
	check func() (T, bool),
) T {
	ctx := context.Background()
	deadline := time.Now().Add(timeout)
	lastNudge := time.Time{}
	var last T
	for {
		v, ok := check()
		last = v
		if ok {
			return last
		}
		if time.Since(lastNudge) > 2*time.Second {
			_ = h.eng.Terminal.Write(ctx, termSessID, []byte("\r"))
			lastNudge = time.Now()
		}
		if time.Now().After(deadline) {
			return last
		}
		time.Sleep(250 * time.Millisecond)
	}
}

// waitForProviderSessionID waits until runnerID has announced a native provider
// conversation — CurrentSession going non-empty, the reducer's "bound" outcome
// having landed — nudging past the CLI's trust dialog in the meantime. It returns
// that conversation id and the runner it read.
//
// It is keyed on the RUNNER, and that is now the only sane way to ask. A runner id
// is minted at spawn and is stable for the whole life of the process, INCLUDING
// across every conversation move it makes — so this one lookup keeps answering after
// a /clear has carried the CLI to a different chat entirely, where the old
// chat-scoped segment scan would have been looking in the wrong place.
//
// (A dead runner reads as ErrNotFound rather than a stale row, so a CLI that died on
// startup shows up here as "never bound" instead of silently passing on a corpse.)
func waitForProviderSessionID(
	t *testing.T,
	h *harness,
	termSessID string,
	runnerID string,
	timeout time.Duration,
) (string, domain.AgentRunner) {
	t.Helper()
	ctx := context.Background()
	var last domain.AgentRunner
	sid := nudgeUntil(h, termSessID, timeout, func() (string, bool) {
		runner, err := h.app.Repositories.AgentRunner.Get(ctx, runnerID)
		if err != nil {
			return "", false // not live (yet, or any more)
		}
		last = runner
		return runner.CurrentSession, runner.CurrentSession != ""
	})
	return sid, last
}

// TestAgent_ClaudeSpawnAndDetect is the must-have acceptance test: it proves
// the daemon's engine/terminal spawns a real `claude` with the descriptor-built
// argv/env, claude's SessionStart hook runs `crowbar hook session_start`, which
// POSTs over the daemon's unix socket to /v0/agent/hooks, which runs
// IngestHook -> the reducer, and the active segment gets bound with a
// non-empty ProviderSessionID — the full Go integration path, end to end.
func TestAgent_ClaudeSpawnAndDetect(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "claude-spawn", repoPath)

	chatID, runnerID, err := h.app.Usecases.Agent.SpawnChat(ctx, wsID, "claude")
	require.NoError(t, err)
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	termSessID := liveRunnerTerminalSession(t, h, chatID)
	require.NotEmpty(t, termSessID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })

	start := time.Now()
	providerSessionID, runner := waitForProviderSessionID(t, h, termSessID, runnerID, 30*time.Second)
	t.Logf("waited %s for SessionStart hook round trip; runner=%+v", time.Since(start), runner)

	require.NotEmpty(t, providerSessionID,
		"timed out after 30s waiting for claude's SessionStart hook to reach /v0/agent/hooks and bind a "+
			"provider conversation; this means either claude never started in the PTY, its SessionStart hook never "+
			"fired, `crowbar hook` could not reach the unix socket, or IngestHook/the reducer did not persist the "+
			"outcome — runner observed: %+v", runner)

	// The runner SpawnChat created is still the one placed on the chat: a bind stays
	// put (it is not a move), and the chat is live because a live-runner row exists.
	require.Equal(t, runnerID, liveRunnerID(t, h, chatID),
		"the chat's live runner must still be the one SpawnChat started")
	require.Equal(t, []string{providerSessionID}, chatSessionIDs(t, h, chatID),
		"the chat must have hosted exactly the one conversation claude announced")
}

// TestAgent_SwitchClaudeToCodex drives one real claude turn (a tiny prompt
// carrying a unique codeword), waits for the resulting Stop hook to append the
// transcript to the chat's ledger, switches the chat to codex, and asserts the
// switch landed: the new active segment is codex, and the assembled handoff
// still carries the codeword from the claude turn (AssembleHandoff reads the
// whole ledger, which switching does not clear). Driving a second real codex
// turn to prove it also detects the codeword is deliberately NOT done here
// (the brief's "deterministic assertion" bar does not require it, and it would
// roughly double the real-API time/cost this test spends for a probabilistic
// assertion on a model reply).
func TestAgent_SwitchClaudeToCodex(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "claude-switch", repoPath)

	chatID, claudeRunnerID, err := h.app.Usecases.Agent.SpawnChat(ctx, wsID, "claude")
	require.NoError(t, err)

	termSessID := liveRunnerTerminalSession(t, h, chatID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })

	start := time.Now()
	providerSessionID, runner := waitForProviderSessionID(t, h, termSessID, claudeRunnerID, 30*time.Second)
	require.NotEmpty(t, providerSessionID, "claude never bound a session before a turn could be driven: %+v", runner)
	t.Logf("claude bound in %s (session=%s)", time.Since(start), providerSessionID)

	const codeword = "FALCON-7719"
	prompt := "Remember this exact codeword for the rest of our conversation: " + codeword +
		". Reply with only the word: acknowledged."
	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte(prompt)))
	// Claude's TUI needs the pasted text to land in the input box before a
	// separate Enter submits it — a trailing \r in the same write is a literal
	// newline inside the box, not a submit (docs/superpowers/specs/
	// spike-2026-07-05-agentic/orchestrator.py found this the hard way).
	time.Sleep(300 * time.Millisecond)
	require.NoError(t, h.eng.Terminal.Write(ctx, termSessID, []byte("\r")))

	start = time.Now()
	handoff := nudgeUntil(h, termSessID, 90*time.Second, func() (string, bool) {
		blob, err := h.app.Usecases.Agent.AssembleHandoff(ctx, chatID)
		require.NoError(t, err)
		return blob, blob != ""
	})
	t.Logf("waited %s for the Stop hook's ledger append", time.Since(start))
	require.NotEmpty(t, handoff, "timed out waiting for a turn_stop hook (ledger append) after driving a real claude turn")
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	newRunnerID, err := h.app.Usecases.Agent.SwitchProvider(ctx, chatID, "codex")
	require.NoError(t, err)
	require.NotEmpty(t, newRunnerID)

	// No wait: the runner Start is asynx SendWait, so the moment SwitchProvider hands
	// back a runner id that runner is durable and visible to the read model.
	newTermSessID := runnerTerminalSession(t, h, newRunnerID)
	require.NotEqual(t, termSessID, newTermSessID, "a provider switch must spawn a NEW PTY for the incoming CLI")
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), newTermSessID) })

	// This is the novel dual-provider-parity claim: not just that the switch
	// mechanically spawned a new PTY and recorded a codex runner, but that codex's OWN
	// SessionStart hook shells out to `crowbar hook session_start`, reaches the daemon
	// over the same unix socket, and the reducer binds a conversation to the NEW runner
	// — exactly as Test A proves for claude.
	codexStart := time.Now()
	codexProviderSessionID, codexRunner := waitForProviderSessionID(t, h, newTermSessID, newRunnerID, 30*time.Second)
	t.Logf("codex bound in %s (session=%s)", time.Since(codexStart), codexProviderSessionID)
	require.NotEmpty(t, codexProviderSessionID,
		"timed out after 30s waiting for codex's SessionStart hook to reach /v0/agent/hooks and bind a "+
			"conversation to the switched-to runner; this means either codex never started in the new PTY, "+
			"its SessionStart hook never fired, `crowbar hook` could not reach the unix socket, or IngestHook/the "+
			"reducer did not persist the outcome — runner observed: %+v", codexRunner)

	require.Equal(t, "codex", codexRunner.ProviderID, "the switched-to runner must be codex")
	require.Equal(t, chatID, codexRunner.CurrentChatID,
		"a provider switch does not move the CHAT: the new CLI is placed on the SAME chat")
	require.Equal(t, newRunnerID, liveRunnerID(t, h, chatID),
		"the chat's live runner must now be the codex one — and the outgoing claude CLI must be gone from it")

	postSwitchHandoff, err := h.app.Usecases.Agent.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	require.Contains(t, postSwitchHandoff, codeword, "handoff after switching to codex must still carry claude's turn")
}
