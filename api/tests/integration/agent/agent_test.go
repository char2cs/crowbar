//go:build integration

// Package agent_test is the acceptance test for the agentic engine (Task 19):
// it drives the REAL claude/codex CLIs through Crowbar's Go stack end to end.
//
// The one thing kit.BuildEnv cannot exercise is the hook round trip: a spawned
// CLI POSTs each hook to the daemon's loopback hook server (claude natively over
// HTTP, codex through curl), at the URL the daemon published under its home. So
// this package builds its own from-scratch daemon (engine -> adapter -> app ->
// v0 router), serves it on the real unix socket and on a published hook server,
// wires a real built `crowbar` binary for the CLIs' `crowbar mcp` (engine.Container.CrowbarBin),
// and drives the agent usecase directly (spawnChat / SwitchProvider) rather than
// through HTTP — the point of the test is proving the hook comes BACK, not
// re-testing the REST surface.
package agent_test

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/char2cs/crowbar/api/internal/adapter"
	v0 "github.com/char2cs/crowbar/api/internal/api/v0"
	"github.com/char2cs/crowbar/api/internal/app"
	agentusecase "github.com/char2cs/crowbar/api/internal/app/usecases/chat"
	"github.com/char2cs/crowbar/api/internal/app/usecases/workspace"
	"github.com/char2cs/crowbar/api/internal/core/gateway/transports"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/engine"
	"github.com/char2cs/crowbar/api/internal/engine/agents"
	"github.com/char2cs/crowbar/api/internal/testutil/accessfake"
	"github.com/char2cs/crowbar/api/tests/kit"
)

// TestMain silences logs and enables gin test mode like every other integration
// package. A vendor CLI records trust by place in its own home, so newHarness
// points both provider homes at temp dirs before anything can spawn one.
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
	// hooks fires after every COMPLETED hook POST. It is this suite's
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

// newHarness builds a real crowbar binary, hands it to the engine over a fresh
// temp CROWBAR_HOME, and serves the real v0
// router on the home's unix socket and on the hook server whose URL it
// publishes there, the one a spawned CLI's hooks are rendered to POST to.
func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)

	// PIN THIS SUITE TO THE HOOKS TRANSPORT (engine.Container.NoAPITransport). Every test here drives a vendor CLI by
	// TYPING INTO ITS PTY and asserts on what that CLI's own hook POSTs put
	// in the ledger — the hook round trip to the hook server is the entire reason
	// this package exists (see the package doc).
	//
	// Codex's descriptor is api-transport (descriptors-v3/codex.yaml, runtime.
	// transport: api), and an api-transport spawn forks TWO codexes: a headless
	// `codex app-server` that owns the chat's conversation, and the disconnected
	// companion PTY spawnRunner still manages alongside it (runner/attach.go calls
	// that a known gap; HasLiveAPIConnection's own doc is where production tells the
	// two apart). Left on, the app-server binds the chat to ITS OWN thread within
	// a second of the spawn — so a turn typed into the companion PTY announces a
	// DIFFERENT session id, and the two CLIs fight over the chat's binding instead of
	// recording the conversation under test.
	//
	// Off, codex runs exactly as design spec §2.2b requires a provider whose api
	// transport is unavailable to run — over hooks alone — the PTY is the ONLY codex
	// and therefore the conversation, and the assertions mean what they say again.
	// This is the same pin internal/app/usecases/chat/harness_test.go applies, for
	// an adjacent reason.

	// Before anything can spawn a CLI: point codex's and claude's OWN homes at
	// throwaway directories, so the trust each one records for this test's temp repo
	// lands there instead of in the user's real config. Both trust barriers still
	// work — see kit.IsolateProviderHomes and firstOfProvider.
	kit.IsolateProviderHomes(t)

	bin := buildCrowbarBinary(t)

	// Released after the adapter's cleanup below (LIFO): the home is held until
	// its stores are closed.
	session, err := kit.BootHome(ctx, home)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, session.Close()) })

	eng, err := engine.New(ctx, engine.WithHomeDir(home))
	require.NoError(t, err)
	eng.CrowbarBin, eng.NoAPITransport = bin, true

	adapters, err := adapter.New(adapter.WithHomeDir(home))
	require.NoError(t, err)
	t.Cleanup(func() { _ = adapters.Close() })

	appContainer, err := app.New(ctx, eng, adapters, session, v0.EncodeEntity, v0.NewSidebar)
	require.NoError(t, err)

	router := gin.New()
	// Installed BEFORE Register so it wraps the hook route: it fires once the hook
	// handler chain has fully returned, which is the moment what the hook ingress
	// recorded is committed and readable.
	hooks := newHookBarrier()
	router.Use(hooks.middleware())
	// Installed BEFORE Register for the same reason, and it additionally has to see
	// the request BODY on the way IN — the handler consumes it — so it snapshots and
	// restores it rather than reading it after the fact.
	mcp := newMCPBarrier()
	router.Use(mcp.middleware())
	apiContainer := v0.New(appContainer, eng, accessfake.Closed())
	apiContainer.Register(router.Group("/v0"))

	ln, err := transports.NewSocket("unix://")
	require.NoError(t, err, "bind daemon unix socket (must be the same path `crowbar mcp` resolves via CROWBAR_HOME)")
	go func() { _ = http.Serve(ln, router) }()
	// Published before anything can spawn a CLI, as the daemon does: the spawner
	// reads it to render the CLI's hook URLs.
	hookServer, err := hookserver.Listen(ctx, router)
	require.NoError(t, err)
	require.NoError(t, hookserver.Publish(home, hookServer.URL()))
	go func() { _ = hookServer.Serve() }()

	// Registered LAST so it runs FIRST (t.Cleanup is LIFO): stop accepting hook
	// traffic and tear down live realtime goroutines before the adapter's
	// SQLite handles (registered above, so it closes after) and t.TempDir's
	// RemoveAll run. Callers that spawn a vendor CLI must additionally
	// t.Cleanup a Terminal.Kill of its session BEFORE calling newHarness's
	// caller returns, so that cleanup (registered later still) runs before
	// even this one and the CLI process is dead before its cwd's temp tree is
	// removed (the terminal suite's documented worktree-busy flake).
	t.Cleanup(func() {
		_ = hookServer.Close()
		_ = ln.Close()
		appContainer.Close()
	})

	return &harness{home: home, app: appContainer, eng: eng, hooks: hooks, mcp: mcp, trusted: map[string]bool{}}
}

// buildCrowbarBinary compiles a real ./cmd/crowbar binary so the daemon's
// spawned claude/codex CLI has a `crowbar mcp` to launch. kit.BuildEnv's
// httptest server never needed this binary because it never spawns a CLI.
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
// agentWorkspaceReader) derives spawnChat's cwd from the workspace row's stored
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

	project, err := h.app.Usecases.ProjectImport.Create(ctx, "", name, repoPath)
	require.NoError(t, err, "create project")

	repo, err := h.app.Usecases.ProjectImport.ImportRepo(ctx, kit.RepoImport(project.ID, repoPath), kit.RequireBranches(t))
	require.NoError(t, err, "import repo")

	ws, err := h.app.Usecases.Workspace.CreateChild(ctx, workspace.CreateChildInput{
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

// requireCLI skips the test when name is not runnable from this process, or
// holds no credential for the real model turn every test here drives.
// exec.LookPath is tried first (the PATH the go test process actually has);
// ~/.local/bin/<name> is this machine's documented fallback location.
func requireCLI(t *testing.T, name string) {
	t.Helper()
	kit.RequireProviderLogin(t, name)
	if _, err := exec.LookPath(name); err == nil {
		return
	}
	fallback := filepath.Join(os.Getenv("HOME"), ".local", "bin", name)
	if _, err := os.Stat(fallback); err == nil {
		return
	}
	t.Skipf("%s not installed", name)
}

// spawnChat mints a chat in wsID and starts provider on it, as a create then a
// Start session do.
func spawnChat(ctx context.Context, h *harness, wsID, provider string) (string, string, error) {
	chatID, err := h.app.Usecases.AgentChat.MintChat(ctx, "", wsID, provider, "")
	if err != nil {
		return "", "", err
	}
	runnerID, err := h.app.Usecases.AgentRunner.StartRunner(ctx, chatID, provider)
	return chatID, runnerID, err
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

// runnerByID is runnerID's runner while the supervisor holds it, also while it
// is exiting.
func runnerByID(h *harness, runnerID string) (agents.Runner, bool) {
	return h.app.Usecases.AgentRunner.(*agentusecase.Usecase).RunnerByID(runnerID) //nolint:forcetypeassert // the harness wires the concrete usecase
}

// runnerTerminalSession returns runnerID's PTY. A plain read: a runner id
// SwitchProvider returned is already in the supervisor's live table.
func runnerTerminalSession(t *testing.T, h *harness, runnerID string) string {
	t.Helper()
	runner, ok := runnerByID(h, runnerID)
	require.True(t, ok, "runner %s is not live", runnerID)
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
	if errors.Is(err, agentusecase.ErrNoLiveRunner) {
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
// argv/env, claude's SessionStart hook POSTs to the daemon's hook server, whose
// hook route runs
// the hook ingress, and the chat's conversation gets bound with a
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
		"claude's SessionStart hook never reached the daemon's hook route to bind a provider conversation; this means "+
			"either claude never started in the PTY, its SessionStart hook never fired, its hook POST could not "+
			"reach the hook server, or the hook ingress did not record the outcome — runner observed: %+v",
		runner)

	// The runner spawnChat started is still the one placed on the chat: a bind stays
	// put (it is not a move), and the chat is live because a live-runner row exists.
	require.Equal(t, runnerID, liveRunnerID(t, h, chatID),
		"the chat's live runner must still be the one spawnChat started")
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

	newRunnerID, err := h.app.Usecases.AgentRunner.SwitchProvider(ctx, chatID, "codex", "")
	require.NoError(t, err)
	require.NotEmpty(t, newRunnerID)

	// No wait: the runner Start is asynx SendWait, so the moment SwitchProvider hands
	// back a runner id that runner is durable and visible to the read model.
	newTermSessID := runnerTerminalSession(t, h, newRunnerID)
	require.NotEqual(t, termSessID, newTermSessID, "a provider switch must spawn a NEW PTY for the incoming CLI")
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), newTermSessID) })

	// This is the novel dual-provider-parity claim: not just that the switch
	// mechanically spawned a new PTY and recorded a codex runner, but that codex's OWN
	// SessionStart hook reaches the daemon's hook server through curl, and the
	// reducer binds a conversation to the NEW runner
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
		"codex's SessionStart hook never reached the daemon's hook route to bind a conversation to the switched-to "+
			"runner; this means either codex never started in the new PTY, its SessionStart hook never fired, "+
			"its hook POST could not reach the hook server, or the hook ingress did not record the "+
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

// TestAgent_CodexTurnAppendsLedger is codex's counterpart to the usecase's
// in-process turn_stop tests (internal/app/usecases/chat/turn_test.go), which
// only ever place a SYNTHETIC hook on a "claude" runner. It has never been proven that a REAL
// codex turn_stop hook (codex's own curl to the hook server, rendered into
// its config by the codex.yaml descriptor) reaches the daemon and produces a real ledger entry from codex's own rollout
// transcript. This spawns codex directly (no switch involved), drives one
// tiny real turn, and asserts: the resulting ledger entry is non-empty (via
// AssembleHandoff); a codex-tagged ASSISTANT .turn entry specifically exists
// physically on disk — the actual proof that codex's turn_stop hook (not its
// UserPromptSubmit hook, which separately writes a codex-tagged USER turn for
// the driven prompt and alone could satisfy a role-blind check) appended a
// ledger entry; and the driven codeword round-trips onto some codex-tagged
// turn on disk (that UserPromptSubmit-recorded user turn, which echoes the
// prompt verbatim — codex was asked to reply with only "acknowledged", so its
// own assistant text never contains the codeword).
func TestAgent_CodexTurnAppendsLedger(t *testing.T) {
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-turn", repoPath)

	// A bare codex spawn paints an interactive trust dialog — a Crowbar-managed
	// child workspace is a git worktree, which codex's trust check treats as "a
	// subdirectory of a Git project" — and emits NO hook traffic behind it. So the
	// dialog is what has to be waited for FIRST, and it is a real, observable thing
	// to wait for: spawnReady blocks until codex has painted it, then dismisses it
	// with one Enter.
	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "codex")
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)
	t.Logf("spawned codex: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	const codeword = "MERIDIAN-2260"
	// Text and the submitting Enter are separate writes (a trailing \r in the same
	// write as pasted text lands as a literal newline inside the input box, not a
	// submit — codex's TUI is Ink-style too), and drive blocks on codex ECHOING the
	// text into its composer in between. That echo is also what proves codex has
	// finished booting its MCP servers and is accepting input at all: if the text
	// had landed in a splash screen it would never come back.
	drive(t, h, tap, termSessID, "Remember this exact codeword for the rest of our conversation: "+codeword+
		". Reply with only the word: acknowledged.")

	providerSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID,
		"codex's SessionStart hook never reached the daemon's hook route to bind a conversation; this means either "+
			"codex never started in the PTY, its SessionStart hook never fired, its hook POST could not reach "+
			"the hook server, or the hook ingress did not record the outcome — runner observed: %+v", runner)

	// AssembleHandoff's rendered blob goes non-empty as soon as ANY ledger turn
	// exists for this chat — including the codex-tagged USER turn its OWN
	// UserPromptSubmit hook writes for the prompt we just drove — so waiting on
	// blob != "" is not a wait for the turn_stop hook at all: it is satisfied in
	// well under a second, long before codex's real model reply and Stop hook run.
	// Wait for a codex-tagged ASSISTANT turn specifically — the actual recorded
	// signal that turn_stop -> AppendTurn ran. An earlier version of this test
	// waited on the blob alone and read the record once right after, which raced
	// the real Stop hook and would have passed even if turn_stop never appended an
	// assistant entry.
	awaitHook(t, h, "codex to append an ASSISTANT ledger turn", func() (bool, bool) {
		ok := len(assistantReplies(readLedgerTurns(t, h, wsID, chatID), "codex")) > 0
		return ok, ok
	})
	turns := readLedgerTurns(t, h, wsID, chatID)
	require.NotEmpty(t, assistantReplies(turns, "codex"),
		"codex's turn_stop hook never appended an ASSISTANT turn after a real codex turn; this "+
			"proves codex's own Stop hook never reached the daemon's hook route, or turn_stop -> AppendTurn never "+
			"ran; turns observed: %+v", turns)

	handoff, err := h.app.Usecases.AgentChat.AssembleHandoff(ctx, chatID)
	require.NoError(t, err)
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	// The doc comment's second half: the entry is DURABLY RECORDED, tagged with
	// the codex provider id. The wait above already proved a codex-tagged
	// ASSISTANT turn exists — the actual proof this test is named for, that
	// codex's turn_stop hook (not its UserPromptSubmit hook, which separately
	// writes a codex-tagged USER turn carrying our full prompt) appended a turn.
	// What remains is a content round-trip check: the driven codeword must land
	// on SOME codex-tagged turn in the record (that UserPromptSubmit-recorded
	// user turn, which echoes the prompt verbatim — codex was asked to reply with
	// only "acknowledged", so its own assistant text never contains the codeword,
	// and this check is deliberately not scoped to the assistant turn).
	var codexTagged, carriesCodeword bool
	for _, tn := range turns {
		if tn.Provider != "codex" {
			continue
		}
		codexTagged = true
		if strings.Contains(tn.Text, codeword) {
			carriesCodeword = true
		}
	}
	require.True(t, codexTagged,
		"a turn must be durably recorded tagged with the codex provider id; turns=%+v", turns)
	require.True(t, carriesCodeword,
		"the codeword must round-trip onto some codex-tagged turn in the record (the UserPromptSubmit-recorded "+
			"user turn, which echoes the driven prompt verbatim); turns=%+v", turns)
}

// readLedgerTurns returns chatID's whole conversation in order, from the rows
// Crowbar recorded off the CLI's own hooks: the oracle for what each side
// said. A chat whose workspace has gone is a fixture fault, failed on here.
func readLedgerTurns(t *testing.T, h *harness, wsID, chatID string) []ledgerTurn {
	t.Helper()
	ctx := context.Background()
	_, err := h.app.Repositories.Workspace.Get(ctx, wsID)
	require.NoError(t, err, "resolve workspace %s the chat is expected on", wsID)

	var turns []ledgerTurn
	for _, m := range kit.Said(chatRows(t, h, chatID)) {
		turns = append(turns, ledgerTurn{Role: m.Role, Provider: m.Provider, Text: m.Text})
	}
	return turns
}

// ledgerTurn is the slice of a recorded turn these tests assert on: who spoke,
// which provider produced it, and what was said.
type ledgerTurn struct {
	Role     string // "user" | "assistant"
	Provider string // the provider of the RUNNER that produced the turn
	Text     string
}

// assistantReplies returns, in order, the text of every ASSISTANT turn the
// given provider produced — the model's OWN Stop-hook output, isolated from
// echoed user prompts and handoff text. This is the record analogue of the old
// per-vendor transcript readers, but it reads Crowbar's own record and can
// attribute a reply to the exact provider that generated it — which the
// rendered AssembleHandoff blob (every turn flattened into one string) cannot.
func assistantReplies(turns []ledgerTurn, provider string) []string {
	var out []string
	for _, tn := range turns {
		if tn.Role == "assistant" && tn.Provider == provider {
			out = append(out, tn.Text)
		}
	}
	return out
}

// seedClaudeThenSwitchToCodex spawns claude, seeds it with codeword via one
// real turn, waits for the resulting ledger entry, switches the chat to
// codex, and brings the incoming codex up to a drivable state — exactly the
// sequence TestAgent_SwitchClaudeToCodex proves, factored out so the gap 3/4
// tests below don't each re-derive ~30 lines of setup. Returns the chat id,
// the claude runner id and the conversation it bound, the new codex runner id,
// the codex runner's terminal session id, and a tap on its PTY.
//
// The incoming codex needs its trust dialog dismissed exactly as a bare spawn
// does. It did not used to: the handoff was injected as codex's POSITIONAL
// prompt, so it sat pre-loaded in the input box and a stray nudge Enter submitted
// it as codex's opening turn. That was the bug — codex answered Crowbar's handoff
// on sight instead of waiting for the user. A switched-to codex now receives the
// handoff through the silent `developer_instructions` config channel (codex.yaml's
// context_inject) and comes up IDLE with an empty composer.
func seedClaudeThenSwitchToCodex(
	t *testing.T,
	h *harness,
	wsID string,
	codeword string,
) (chatID, claudeRunnerID, claudeSessionID, codexRunnerID, codexTermSessID string, codexTap *kit.PTYTap) {
	t.Helper()

	chatID, claudeRunnerID, claudeTermSessID, claudeTap := spawnReady(t, h, wsID, "claude")

	claudeSessionID, runner := awaitSessionBound(t, h, claudeRunnerID, claudeTermSessID, claudeTap)
	require.NotEmpty(t, claudeSessionID, "claude never bound a session before a turn could be driven: %+v", runner)

	drive(t, h, claudeTap, claudeTermSessID, "Remember this exact codeword for the rest of our conversation: "+
		codeword+". Reply with only the word: acknowledged.")

	// claude must FINISH its turn before it is switched away from: the codeword is in
	// the ledger the moment the prompt is submitted, so that alone would switch mid-answer.
	awaitTurnComplete(t, h, wsID, chatID, "claude")
	handoff := awaitHandoffContains(t, h, chatID, codeword)
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	// Bring codex past its trust dialog — and deliberately do NOT wait for it to bind
	// a conversation. It cannot be waited for: codex creates its rollout, and
	// therefore fires SessionStart, LAZILY on its first real turn, and a switched-to
	// codex is by design IDLE — the handoff reached it through the silent
	// developer_instructions channel, so it has an empty composer and nothing to
	// answer. Waiting for a session id here would be waiting on codex's own laziness
	// and would never return. Callers that need codex's conversation id must drive a
	// turn FIRST and await the bind after (TestAgent_CodexTurnAppendsLedger is the
	// model).
	codexRunnerID, codexTermSessID, codexTap = switchTo(t, h, chatID, "codex")
	require.NotEqual(t, claudeTermSessID, codexTermSessID, "a switch must spawn a NEW PTY for the incoming codex CLI")

	// The switched-to codex must be IDLE, not answering the handoff: the document
	// went in through developer_instructions, which is not a user message, so codex
	// has nothing to reply to and the ledger has no codex turn.
	//
	// This is asserted at the one moment it is meaningful and provable: codex has just
	// told us, by painting its composer prompt, that it is up and waiting for input —
	// so "no codex turn in the ledger" is codex genuinely declining to answer the
	// handoff, not merely us looking too early. (The old code reached for the same
	// teeth by firing Enters at it for 30 seconds; being able to observe readiness
	// directly is strictly stronger, and does not type into the user's composer.)
	require.Empty(t, assistantReplies(readLedgerTurns(t, h, wsID, chatID), "codex"),
		"a switched-to codex must not answer the handoff on sight — it is context, not a prompt")

	return chatID, claudeRunnerID, claudeSessionID, codexRunnerID, codexTermSessID, codexTap
}

// TestAgent_CodexUsesHandoff closes gap 3 of the Priority-1 e2e audit: proving
// the RECEIVING provider actually USES a cross-provider handoff, not merely
// that the handoff STRING was passed. TestAgent_SwitchClaudeToCodex
// deliberately stops at asserting AssembleHandoff's blob contains the
// codeword (its own comment explains why: driving a second real codex turn
// "would roughly double the real-API time/cost ... for a probabilistic
// assertion on a model reply"). This test pays that cost to close the gap:
// after a real claude->codex switch (via seedClaudeThenSwitchToCodex), it
// asks codex directly what codeword appeared in the context it was given, and
// asserts CODEX'S OWN REPLY (isolated from the echoed user/handoff turns by
// reading only assistant turns tagged with the codex provider in Crowbar's
// ledger — codex's own Stop-hook output) contains it — the Go-stack proof of
// the Phase-0 spike's "Codex read Claude's raw 38 KB .jsonl and extracted the
// codeword" finding (docs/superpowers/specs/
// 2026-07-05-crowbar-agentic-engine-design.md §1 scorecard).
//
// The ledger — not a vendor transcript — is the v2 oracle. AssembleHandoff's
// rendered blob is NOT usable for this check: it flattens EVERY turn (claude's
// seeding turn, the echoed handoff, the follow-up prompt) into one string that
// already contains the codeword, so "handoff contains codeword" would be
// trivially true without proving codex generated anything. Reading only the
// codex-tagged ASSISTANT turns isolates codex's own model output.
func TestAgent_CodexUsesHandoff(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-handoff", repoPath)

	const codeword = "OSPREY-4482"
	chatID, _, _, _, codexTermSessID, codexTap := seedClaudeThenSwitchToCodex(t, h, wsID, codeword)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	// seedClaudeThenSwitchToCodex has already asserted the switched-to codex is
	// IDLE — it has produced no turn, because the handoff reached it through
	// developer_instructions rather than as its opening user prompt. So codex's
	// FIRST assistant turn is necessarily a reply to what WE type, and it can
	// only carry the codeword if codex actually read the injected context. That
	// makes the old baseline-counting dance (needed when codex auto-answered the
	// handoff, and which once false-passed by matching the auto-turn's own reply)
	// unnecessary: there is nothing to race against.
	drive(t, h, codexTap, codexTermSessID,
		"What exact codeword appeared in the context you were given? Reply with only that word.")

	replies := awaitAssistantReply(t, h, wsID, chatID, "codex", codeword)
	require.NotEmpty(t, replies, "codex never replied referencing the codeword")
	require.Contains(t, strings.Join(replies, "\n"), codeword,
		"codex's own reply must reference the codeword handed off from claude's session — proves codex "+
			"actually READ the handoff injected via developer_instructions, not just that the string was "+
			"passed to it; codex's replies were: %q", replies)

	// The injected handoff must never be recorded as something the USER said: that
	// is what made each handoff nest inside the next one.
	for _, turn := range readLedgerTurns(t, h, wsID, chatID) {
		if turn.Role == "user" {
			require.NotContains(t, turn.Text, "HANDED-OFF CONTEXT",
				"Crowbar's own handoff document must never be recorded as a user turn")
		}
	}
}

// TestAgent_SwitchBackRestoresClaudeContext is the best-effort gap 4 of the
// Priority-1 e2e audit: codex->claude switch-back via `--resume`, proving the
// returning claude restores its native context. It combines two proofs:
//
//  1. Deterministic/cheap: the resumed segment's ProviderSessionID must equal
//     the ORIGINAL (pre-switch) claude segment's — i.e. SwitchProvider's
//     `--resume <id>` (internal/app/usecases/chat/internal/runner's priorSessionID
//     lookup + descriptor session.resume) actually reattached claude to its own
//     prior native session, the Go-stack proof of the Phase-0 spike's "Native
//     resume / Case-1 (--resume <id> -> source=resume)" scorecard row. Under
//     descriptor-v2 the session id is the continuity witness (Crowbar no longer
//     records a vendor transcript path). This alone needs no model turn at all.
//  2. Behavioural: ask the resumed claude to recall the codeword seeded
//     before the first switch, and check its reply — read from claude's own
//     Stop-hook assistant turns in Crowbar's ledger. This IS now a clean
//     isolation of "answered purely from native --resume". It used not to be:
//     SwitchProvider re-appended the WHOLE ledger on every switch, including a
//     switch-back, so the codeword was handed straight back to claude in its
//     system prompt and the reply proved nothing. A provider resumed into its
//     own session is now handed only the GAP — what happened while it was away —
//     and nothing happened here (codex bound a session and was switched away
//     from without taking a turn), so the gap is EMPTY: claude is spawned with
//     no conversation document at all. The only place the codeword can come from
//     is claude's own resumed session.
//  3. Best-effort: no synthetic housekeeping reply, the one signal left that the
//     switch's graceful terminate let the outgoing claude exit cleanly.
func TestAgent_SwitchBackRestoresClaudeContext(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "switch-back", repoPath)

	const codeword = "TALON-6631"
	chatID, _, origClaudeSessionID, _, codexTermSessID, _ := seedClaudeThenSwitchToCodex(t, h, wsID, codeword)
	// seedClaudeThenSwitchToCodex leaves codex live and active; switching back
	// below kills it, but guard the cleanup in case the switch itself fails.
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	// claude's original conversation is now part of the CHAT's append-only history —
	// it outlives the runner that opened it, which is exactly what a resume needs.
	require.NotEmpty(t, origClaudeSessionID, "the original claude runner must have bound a session id")
	require.Contains(t, chatSessionIDs(t, h, chatID), origClaudeSessionID,
		"the chat's conversation history must still carry claude's conversation after it was switched away from")

	// The resumed claude is a FRESH PROCESS in a fresh PTY, so it shows its trust
	// dialog again and — exactly as at first spawn — fires no SessionStart until it
	// is dismissed. Blocking on that dialog is what makes the bind below arrive.
	newClaudeRunnerID, newClaudeTermSessID, newClaudeTap := switchTo(t, h, chatID, "claude")
	require.NotEqual(t, codexTermSessID, newClaudeTermSessID, "switch-back must spawn a new terminal session")

	resumedProviderSessionID, resumedRunner := awaitSessionBound(t, h, newClaudeRunnerID, newClaudeTermSessID, newClaudeTap)
	require.NotEmpty(t, resumedProviderSessionID,
		"the switched-back claude's SessionStart hook never bound: %+v", resumedRunner)

	require.Equal(t, origClaudeSessionID, resumedProviderSessionID,
		"switch-back must --resume the ORIGINAL claude session id, not mint a new one — this is the "+
			"Go-stack proof of native resume (Phase-0 spike's Case-1)")
	require.Equal(t, newClaudeRunnerID, liveRunnerID(t, h, chatID),
		"the resumed claude CLI must be the one placed on the chat")

	// A resumed claude given a freshly `--append-system-prompt`-ed delta may
	// emit a synthetic "Continue from where you left off." / "No response
	// requested." housekeeping turn before anything we type is processed
	// (observed live across repeated runs pre-graceful-terminate). Whether such
	// a turn even reaches Crowbar's ledger depends on whether it fires claude's
	// Stop hook, which was inconsistent live. Rather than try to name which
	// assistant turn is "the real reply" (an earlier version raced on
	// index-counting), drive the follow-up immediately and wait for ANY of
	// claude's own assistant turns in the ledger to contain the codeword —
	// unambiguous, since neither the original "acknowledged" turn nor a "No
	// response requested." housekeeping turn could ever satisfy that on their
	// own. Reading claude-tagged assistant turns from Crowbar's ledger replaces
	// the pre-v2 read of claude's native transcript (which v2 no longer records
	// a path to).
	drive(t, h, newClaudeTap, newClaudeTermSessID,
		"What was the codeword I asked you to remember earlier in our conversation? Reply with only that word.")

	texts := awaitAssistantReply(t, h, wsID, chatID, "claude", codeword)
	require.Contains(t, strings.Join(texts, "\n"), codeword,
		"no claude assistant turn (including the follow-up's) referenced the codeword; turns observed: %q", texts)
	// Best-effort: a synthetic "No response requested." housekeeping reply means
	// the outgoing claude was killed mid-flight instead of exiting on SIGTERM.
	for _, tx := range texts {
		assert.NotContains(t, strings.ToLower(tx), "no response requested",
			"a synthetic housekeeping reply means claude did not exit cleanly; turns observed: %q", texts)
	}
}

// switchTo switches chatID to provider and brings the new CLI past its trust
// dialog; the new PTY is killed with the test.
func switchTo(t *testing.T, h *harness, chatID, provider string) (runnerID, termSessID string, tap *kit.PTYTap) {
	t.Helper()
	runnerID, err := h.app.Usecases.AgentRunner.SwitchProvider(context.Background(), chatID, provider, "")
	require.NoError(t, err)
	termSessID = runnerTerminalSession(t, h, runnerID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })
	return runnerID, termSessID, attachReady(t, h, termSessID, provider, runnerID)
}

// TestAgent_SwitchBackToCodexResumesItsOwnSession closes the counterpart of gap 4
// for codex, and exists because the app shipped a bug that only a REAL codex could
// show: switching back to codex was impossible.
//
// Crowbar used to point CODEX_HOME at a directory it owned and deleted, which made
// it the custodian of codex's SESSIONS — codex keeps its rollouts inside its home,
// and `codex resume <id>` has nothing else to go on. Leaving codex therefore
// DESTROYED codex's own session: coming back resumed a thread that no longer
// existed, the CLI died on startup ("no rollout found for thread id ..."), its
// segment ended seconds later, and the chat could never return to codex. Every unit
// test passed — the deletion is real and the resume arg is correct; only the vendor
// CLI knows the thread is gone.
//
// Crowbar no longer owns codex's home: a provider owns its own sessions, and Crowbar
// injects its hooks as config overrides instead. This drives that for real — seed
// codex with a codeword, switch to claude, switch BACK — and requires the returning
// codex to (a) resume the SAME native session id and (b) still be ALIVE and
// answering from its own restored session (the gap it is handed carries claude's
// turns, not codex's own).
func TestAgent_SwitchBackToCodexResumesItsOwnSession(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-switch-back", repoPath)

	const codeword = "PANGOLIN-9931"

	// Clear the trust dialog, then DRIVE A TURN, and only then wait for the
	// conversation to bind. The order is forced by codex, not by taste: it creates
	// its rollout — and so fires SessionStart — lazily, on its first real turn.
	// Waiting for a session id on a codex that has not spoken yet waits forever
	// (TestAgent_CodexTurnAppendsLedger drives first, which is why it passes).
	chatID, codexSegID, codexTermSessID, codexTap := spawnReady(t, h, wsID, "codex")
	diagnoseOnFailure(t, h, codexTap, "codex")

	drive(t, h, codexTap, codexTermSessID, "Remember this exact codeword for the rest of our conversation: "+
		codeword+". Reply with only the word: acknowledged.")

	origSessionID, runner := awaitSessionBound(t, h, codexSegID, codexTermSessID, codexTap)
	require.NotEmpty(t, origSessionID, "codex never bound a session even after a real turn was driven: %+v", runner)

	// codex must FINISH its turn before we leave it. This is THE load-bearing barrier of
	// this test: the switch below terminates the outgoing codex, and if it is still
	// composing its reply, that reply never lands — not in the ledger, and not in codex's
	// own rollout. The codex resumed at the end would then have nothing to recall, and the
	// test would fail on its final assertion for a reason four steps upstream.
	awaitTurnComplete(t, h, wsID, chatID, "codex")
	blob := awaitHandoffContains(t, h, chatID, codeword)
	require.Contains(t, blob, codeword, "codex's turn never reached the ledger")

	// Leave codex. This ends its segment and reaps that segment's tmp dir — the very
	// thing that used to take codex's session with it.
	claudeSegID, claudeTermSessID, claudeLegTap := switchTo(t, h, chatID, "claude")
	// Registered HERE rather than at the top of the test: this is the leg that
	// costs five minutes when it goes wrong, and a backstop expiry inside
	// awaitSessionBound reports only an empty session id — the CLI's own screen is
	// the only thing that says whether claude never started, is parked on a prompt,
	// or is cycling API retries.
	diagnoseOnFailure(t, h, claudeLegTap, "claude")
	claudeSessionID, claudeRunner := awaitSessionBound(t, h, claudeSegID, claudeTermSessID, claudeLegTap)
	require.NotEmpty(t, claudeSessionID, "the switched-to claude never bound a session: %+v", claudeRunner)

	// ...and come back.
	// Drive the follow-up turn FIRST, for the same reason as above: codex announces its
	// conversation on its first turn, not at startup. That turn does double duty — it is
	// also the question whose ANSWER proves the resumed codex still has its own history.
	backSegID, backTermSessID, backTap := switchTo(t, h, chatID, "codex")
	drive(t, h, backTap, backTermSessID, "What was the codeword I asked you to remember? Reply with only that word.")

	resumedSessionID, resumedRunner := awaitSessionBound(t, h, backSegID, backTermSessID, backTap)
	require.NotEmpty(t, resumedSessionID,
		"the switched-back codex never bound a session — it almost certainly died on startup, which is "+
			"what happens when its rollout was deleted with the previous runner: %+v", resumedRunner)
	require.Equal(t, origSessionID, resumedSessionID,
		"switching back to codex must RESUME its original native session, not mint a new one")

	replies := awaitAssistantReply(t, h, wsID, chatID, "codex", codeword)
	require.Contains(t, strings.Join(replies, "\n"), codeword,
		"the resumed codex must answer from its OWN restored session; replies seen: %q", replies)
}
