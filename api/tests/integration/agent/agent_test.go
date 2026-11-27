//go:build integration

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

func TestMain(m *testing.M) {
	kit.Main(m)
}

type harness struct {
	home    string
	app     *app.Container
	eng     *engine.Container
	hooks   *hookBarrier
	mcp     *mcpBarrier
	trusted map[string]bool
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	ctx := context.Background()

	home := t.TempDir()
	t.Setenv("CROWBAR_HOME", home)

	kit.IsolateProviderHomes(t)

	bin := buildCrowbarBinary(t)

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
	hooks := newHookBarrier()
	router.Use(hooks.middleware())
	mcp := newMCPBarrier()
	router.Use(mcp.middleware())
	apiContainer := v0.New(appContainer, eng, accessfake.Closed())
	apiContainer.Register(router.Group("/v0"))

	ln, err := transports.NewSocket("unix://")
	require.NoError(t, err, "bind daemon unix socket (must be the same path `crowbar mcp` resolves via CROWBAR_HOME)")
	go func() { _ = http.Serve(ln, router) }()
	hookServer, err := hookserver.Listen(ctx, router)
	require.NoError(t, err)
	require.NoError(t, hookserver.Publish(home, hookServer.URL()))
	go func() { _ = hookServer.Serve() }()

	t.Cleanup(func() {
		_ = hookServer.Close()
		_ = ln.Close()
		appContainer.Close()
	})

	return &harness{home: home, app: appContainer, eng: eng, hooks: hooks, mcp: mcp, trusted: map[string]bool{}}
}

func buildCrowbarBinary(t *testing.T) string {
	t.Helper()
	bin := filepath.Join(t.TempDir(), "crowbar")
	cmd := exec.Command("go", "build", "-tags", "noEmbed", "-o", bin, "./cmd/crowbar")
	cmd.Dir = apiRootDir(t)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "build crowbar binary: %s", out)
	return bin
}

func apiRootDir(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "resolve test file path")
	return filepath.Join(filepath.Dir(file), "..", "..", "..")
}

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

func spawnChat(ctx context.Context, h *harness, wsID, provider string) (string, string, error) {
	chatID, err := h.app.Usecases.AgentChat.MintChat(ctx, "", wsID, provider, "")
	if err != nil {
		return "", "", err
	}
	runnerID, err := h.app.Usecases.AgentRunner.StartRunner(ctx, chatID, provider)
	return chatID, runnerID, err
}

func liveRunnerTerminalSession(t *testing.T, h *harness, chatID string) string {
	t.Helper()
	runner, err := h.app.Usecases.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
	require.NoError(t, err, "chat %s has no live runner (its CLI never started, or died immediately)", chatID)
	require.NotEmpty(t, runner.TerminalSession)
	return runner.TerminalSession
}

func runnerByID(h *harness, runnerID string) (agents.Runner, bool) {
	return h.app.Usecases.AgentRunner.(*agentusecase.Usecase).RunnerByID(runnerID) //nolint:forcetypeassert // the harness wires the concrete usecase
}

func runnerTerminalSession(t *testing.T, h *harness, runnerID string) string {
	t.Helper()
	runner, ok := runnerByID(h, runnerID)
	require.True(t, ok, "runner %s is not live", runnerID)
	require.NotEmpty(t, runner.TerminalSession)
	return runner.TerminalSession
}

func liveRunnerID(t *testing.T, h *harness, chatID string) string {
	t.Helper()
	runner, err := h.app.Usecases.AgentRunner.LiveRunnerForChat(context.Background(), chatID)
	if errors.Is(err, agentusecase.ErrNoLiveRunner) {
		return ""
	}
	require.NoError(t, err)
	return runner.ID
}

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

func TestAgent_ClaudeSpawnAndDetect(t *testing.T) {
	requireCLI(t, "claude")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "claude-spawn", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "claude")
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)
	t.Logf("spawned claude: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	providerSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID,
		"claude's SessionStart hook never reached the daemon's hook route to bind a provider conversation; this means "+
			"either claude never started in the PTY, its SessionStart hook never fired, its hook POST could not "+
			"reach the hook server, or the hook ingress did not record the outcome — runner observed: %+v",
		runner)

	require.Equal(t, runnerID, liveRunnerID(t, h, chatID),
		"the chat's live runner must still be the one spawnChat started")
	require.Equal(t, []string{providerSessionID}, chatSessionIDs(t, h, chatID),
		"the chat must have hosted exactly the one conversation claude announced")
}

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
	drive(t, h, tap, termSessID, "Remember this exact codeword for the rest of our conversation: "+codeword+
		". Reply with only the word: acknowledged.")

	awaitTurnComplete(t, h, wsID, chatID, "claude")
	handoff := awaitHandoffContains(t, h, chatID, codeword)
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	newRunnerID, err := h.app.Usecases.AgentRunner.SwitchProvider(ctx, chatID, "codex", "")
	require.NoError(t, err)
	require.NotEmpty(t, newRunnerID)

	newTermSessID := runnerTerminalSession(t, h, newRunnerID)
	require.NotEqual(t, termSessID, newTermSessID, "a provider switch must spawn a NEW PTY for the incoming CLI")
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), newTermSessID) })

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

func TestAgent_CodexTurnAppendsLedger(t *testing.T) {
	requireCLI(t, "codex")
	h := newHarness(t)
	ctx := context.Background()

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-turn", repoPath)

	chatID, runnerID, termSessID, tap := spawnReady(t, h, wsID, "codex")
	require.NotEmpty(t, chatID)
	require.NotEmpty(t, runnerID)
	t.Logf("spawned codex: chat=%s runner=%s workspace=%s home=%s", chatID, runnerID, wsID, h.home)

	const codeword = "MERIDIAN-2260"
	drive(t, h, tap, termSessID, "Remember this exact codeword for the rest of our conversation: "+codeword+
		". Reply with only the word: acknowledged.")

	providerSessionID, runner := awaitSessionBound(t, h, runnerID, termSessID, tap)
	require.NotEmpty(t, providerSessionID,
		"codex's SessionStart hook never reached the daemon's hook route to bind a conversation; this means either "+
			"codex never started in the PTY, its SessionStart hook never fired, its hook POST could not reach "+
			"the hook server, or the hook ingress did not record the outcome — runner observed: %+v", runner)

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

type ledgerTurn struct {
	Role     string
	Provider string
	Text     string
}

func assistantReplies(turns []ledgerTurn, provider string) []string {
	var out []string
	for _, tn := range turns {
		if tn.Role == "assistant" && tn.Provider == provider {
			out = append(out, tn.Text)
		}
	}
	return out
}

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

	awaitTurnComplete(t, h, wsID, chatID, "claude")
	handoff := awaitHandoffContains(t, h, chatID, codeword)
	require.Contains(t, handoff, codeword, "ledger blob must carry the turn we just drove")

	codexRunnerID, codexTermSessID, codexTap = switchTo(t, h, chatID, "codex")
	require.NotEqual(t, claudeTermSessID, codexTermSessID, "a switch must spawn a NEW PTY for the incoming codex CLI")

	require.Empty(t, assistantReplies(readLedgerTurns(t, h, wsID, chatID), "codex"),
		"a switched-to codex must not answer the handoff on sight — it is context, not a prompt")

	return chatID, claudeRunnerID, claudeSessionID, codexRunnerID, codexTermSessID, codexTap
}

func TestAgent_CodexUsesHandoff(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-handoff", repoPath)

	const codeword = "OSPREY-4482"
	chatID, _, _, _, codexTermSessID, codexTap := seedClaudeThenSwitchToCodex(t, h, wsID, codeword)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	drive(t, h, codexTap, codexTermSessID,
		"What exact codeword appeared in the context you were given? Reply with only that word.")

	replies := awaitAssistantReply(t, h, wsID, chatID, "codex", codeword)
	require.NotEmpty(t, replies, "codex never replied referencing the codeword")
	require.Contains(t, strings.Join(replies, "\n"), codeword,
		"codex's own reply must reference the codeword handed off from claude's session — proves codex "+
			"actually READ the handoff injected via developer_instructions, not just that the string was "+
			"passed to it; codex's replies were: %q", replies)

	for _, turn := range readLedgerTurns(t, h, wsID, chatID) {
		if turn.Role == "user" {
			require.NotContains(t, turn.Text, "HANDED-OFF CONTEXT",
				"Crowbar's own handoff document must never be recorded as a user turn")
		}
	}
}

func TestAgent_SwitchBackRestoresClaudeContext(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "switch-back", repoPath)

	const codeword = "TALON-6631"
	chatID, _, origClaudeSessionID, _, codexTermSessID, _ := seedClaudeThenSwitchToCodex(t, h, wsID, codeword)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), codexTermSessID) })

	require.NotEmpty(t, origClaudeSessionID, "the original claude runner must have bound a session id")
	require.Contains(t, chatSessionIDs(t, h, chatID), origClaudeSessionID,
		"the chat's conversation history must still carry claude's conversation after it was switched away from")

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

	drive(t, h, newClaudeTap, newClaudeTermSessID,
		"What was the codeword I asked you to remember earlier in our conversation? Reply with only that word.")

	texts := awaitAssistantReply(t, h, wsID, chatID, "claude", codeword)
	require.Contains(t, strings.Join(texts, "\n"), codeword,
		"no claude assistant turn (including the follow-up's) referenced the codeword; turns observed: %q", texts)
	for _, tx := range texts {
		assert.NotContains(t, strings.ToLower(tx), "no response requested",
			"a synthetic housekeeping reply means claude did not exit cleanly; turns observed: %q", texts)
	}
}

func switchTo(t *testing.T, h *harness, chatID, provider string) (runnerID, termSessID string, tap *kit.PTYTap) {
	t.Helper()
	runnerID, err := h.app.Usecases.AgentRunner.SwitchProvider(context.Background(), chatID, provider, "")
	require.NoError(t, err)
	termSessID = runnerTerminalSession(t, h, runnerID)
	t.Cleanup(func() { _ = h.eng.Terminal.Kill(context.Background(), termSessID) })
	return runnerID, termSessID, attachReady(t, h, termSessID, provider, runnerID)
}

func TestAgent_SwitchBackToCodexResumesItsOwnSession(t *testing.T) {
	requireCLI(t, "claude")
	requireCLI(t, "codex")
	h := newHarness(t)

	repoPath := kit.InitRepo(t)
	_, _, wsID := h.importRepoAndWorkspace(t, "codex-switch-back", repoPath)

	const codeword = "PANGOLIN-9931"

	chatID, codexSegID, codexTermSessID, codexTap := spawnReady(t, h, wsID, "codex")
	diagnoseOnFailure(t, h, codexTap, "codex")

	drive(t, h, codexTap, codexTermSessID, "Remember this exact codeword for the rest of our conversation: "+
		codeword+". Reply with only the word: acknowledged.")

	origSessionID, runner := awaitSessionBound(t, h, codexSegID, codexTermSessID, codexTap)
	require.NotEmpty(t, origSessionID, "codex never bound a session even after a real turn was driven: %+v", runner)

	awaitTurnComplete(t, h, wsID, chatID, "codex")
	blob := awaitHandoffContains(t, h, chatID, codeword)
	require.Contains(t, blob, codeword, "codex's turn never reached the ledger")

	claudeSegID, claudeTermSessID, claudeLegTap := switchTo(t, h, chatID, "claude")
	diagnoseOnFailure(t, h, claudeLegTap, "claude")
	claudeSessionID, claudeRunner := awaitSessionBound(t, h, claudeSegID, claudeTermSessID, claudeLegTap)
	require.NotEmpty(t, claudeSessionID, "the switched-to claude never bound a session: %+v", claudeRunner)

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
