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

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	agentrunner "github.com/char2cs/crowbar/api/internal/engine/agents/runner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/worktree"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain silences logs and enables gin test mode like every other integration
// package, and additionally polices the user's REAL provider homes across the run.
//
// This is the only package that spawns real vendor CLIs, and a vendor CLI records
// trust by PLACE in a home Crowbar does not own. Every test here builds a throwaway
// repo, so an un-isolated run appends one permanent, already-dead stanza per test to
// the user's own ~/.codex/config.toml and ~/.claude.json. newHarness isolates both
// homes; this fails the run if anything slipped past that.
func TestMain(m *testing.M) {
	kit.MainGuardingProviderHomes(m)
}

// harness is a from-scratch daemon wired the same way cmd/crowbar's `serve`
// wires it, but pinned to a temp CROWBAR_HOME and served on the real unix
// socket instead of a fixed system path.
type harness struct {
	home string
	app  *app.Container
	eng  *engine.Container
	// hooks fires after every COMPLETED `crowbar hook` POST. It is this suite's
	// synchronisation primitive: a vendor CLI announcing itself is an event, and
	// awaitHook blocks on it instead of re-sampling the read model on a timer.
	hooks *hookBarrier
	// mcp records — and fires on — every MCP message a vendor CLI's own MCP client
	// relays into the daemon. It is both a second wakeup source and the only
	// evidence that the TOOL surface was reached at all: every other titling
	// channel lands the identical field (see mcpBarrier).
	mcp *mcpBarrier
	// trusted records which providers have already answered a trust dialog in this
	// harness's repo. See firstOfProvider: only the FIRST CLI of a provider is shown
	// one, so it is the only one a test may block on.
	trusted map[string]bool
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

	// Before anything can spawn a CLI: point codex's and claude's OWN homes at
	// throwaway directories, so the trust each one records for this test's temp repo
	// lands there instead of in the user's real config. Both trust barriers still
	// work — see kit.IsolateProviderHomes and firstOfProvider.
	kit.IsolateProviderHomes(t)

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
	// Installed BEFORE Register so it wraps the hook route: it fires once the hook
	// handler chain has fully returned, which is the moment IngestHook's effects
	// (the reducer's committed outcome, the ledger turn on disk) are all readable.
	hooks := newHookBarrier()
	router.Use(hooks.middleware())
	// Installed BEFORE Register for the same reason, and it additionally has to see
	// the request BODY on the way IN — the handler consumes it — so it snapshots and
	// restores it rather than reading it after the fact.
	mcp := newMCPBarrier()
	router.Use(mcp.middleware())
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

	return &harness{home: home, app: appContainer, eng: eng, hooks: hooks, mcp: mcp, trusted: map[string]bool{}}
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

	repo, err := h.app.Usecases.ProjectImport.ImportRepo(ctx, project.ID, "", repoPath)
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
	runner, err := h.app.Usecases.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
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
	runner, err := h.app.Usecases.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
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
	convs, err := h.app.Usecases.AgentRunner.ConversationsForChat(context.Background(), chatID)
	require.NoError(t, err)
	out := make([]string, 0, len(convs))
	for _, c := range convs {
		out = append(out, c.SessionID)
	}
	return out
}

// TestAgent_ClaudeSpawnAndDetect is the must-have acceptance test: it proves
// the daemon's core/terminal spawns a real `claude` with the descriptor-built
// argv/env, claude's SessionStart hook runs `crowbar hook session_start`, which
// POSTs over the daemon's unix socket to /v0/agent/hooks, which runs
// IngestHook -> the reducer, and the active segment gets bound with a
// non-empty ProviderSessionID — the full Go integration path, end to end.
func TestAgent_ClaudeSpawnAndDetect(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "claude-spawn", repoPath)

	// spawnReady spawns claude, taps its PTY and blocks until it has PAINTED its
	// trust dialog, which it then dismisses with one Enter. That dialog is not
	// cosmetic: claude fires no SessionStart at all until the folder is trusted, so
	// dismissing it is the precondition for the hook this test is about.
	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	// No turn is driven and nothing is nudged: claude announces itself at boot, and
	// this blocks on that hook ARRIVING rather than on a stopwatch.
	providerSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID,
		"claude's SessionStart hook never reached /v0/agent/hooks to bind a provider conversation; this means "+
			"either claude never started in the PTY, its SessionStart hook never fired, `crowbar hook` could not "+
			"reach the unix socket, or IngestHook/the reducer did not persist the outcome — runner observed: %+v",
		runner)

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

	chatID, claudeRunnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")

	providerSessionID, runner := awaitSessionBound(t, h, claudeRunnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID, "claude never bound a session before a turn could be driven: %+v", runner)

	const codeword = "FALCON-7719"
	// drive blocks on claude's OWN ECHO of the prompt into its composer before it
	// sends the submitting Enter — the app telling us it took the input. The two
	// writes cannot be merged (a trailing \r in the same write is a literal newline
	// inside the box, not a submit), and the gap between them is a real signal, not
	// a 300ms guess.
	drive(t, h, tap, termSessID, "Remember this exact codeword for the rest of our conversation: "+codeword+
		". Reply with only the word: acknowledged.")

	// Block on claude's Stop hook landing the turn in the ledger.
	// Wait for claude to FINISH the turn (its turn_stop hook appending an assistant
	// entry), not merely for the ledger to mention the codeword — the prompt we typed
	// puts the codeword there the instant it is submitted. Switching on the weaker
	// signal would terminate claude mid-answer.
	awaitTurnComplete(t, h, wsID, chatID, "claude")
	handoff := awaitHandoffContains(t, h, chatID, codeword)
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	newRunnerID, err := h.app.Usecases.AgentRunner.SwitchProvider(ctx, chatID, "codex")
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
	//
	// It has to DRIVE A TURN to get there, and cannot simply wait. codex creates its
	// rollout — and so fires SessionStart — LAZILY, on its first real turn, and a
	// switched-to codex comes up IDLE by design: the handoff reaches it through the
	// silent developer_instructions channel, not as a prompt, so it has nothing to
	// answer and nothing to announce. Waiting for a session id from a codex that has
	// not spoken waits forever. (This is also why the test spends one small codex turn
	// it once tried to avoid — that turn is the price of codex announcing itself at
	// all, not an extra assertion.)
	codexTap := attachReady(t, h, newTermSessID, "codex", newRunnerID)
	drive(t, h, codexTap, newTermSessID, "Reply with only the word: ready.")

	codexProviderSessionID, codexRunner := awaitSessionBound(t, h, newRunnerID, newTermSessID, codexTap)
	require.NotEmpty(t, codexProviderSessionID,
		"codex's SessionStart hook never reached /v0/agent/hooks to bind a conversation to the switched-to "+
			"runner; this means either codex never started in the new PTY, its SessionStart hook never fired, "+
			"`crowbar hook` could not reach the unix socket, or IngestHook/the reducer did not persist the "+
			"outcome — runner observed: %+v", codexRunner)

	require.Equal(t, "codex", codexRunner.ProviderID, "the switched-to runner must be codex")
	require.Equal(t, chatID, codexRunner.CurrentChatID,
		"a provider switch does not move the CHAT: the new CLI is placed on the SAME chat")
	require.Equal(t, newRunnerID, liveRunnerID(t, h, chatID),
		"the chat's live runner must now be the codex one — and the outgoing claude CLI must be gone from it")

	postSwitchHandoff, err := h.app.Usecases.AgentChat.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	require.Contains(t, postSwitchHandoff, codeword, "handoff after switching to codex must still carry claude's turn")
}
