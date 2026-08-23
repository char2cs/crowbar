package agent

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/repositories/agentchat"
	"github.com/char2cs/crowbar/api/internal/app/repositories/agentrunner"
	"github.com/char2cs/crowbar/api/internal/app/usecases/internal/worktreepath"
	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
	engineterminal "github.com/char2cs/crowbar/api/internal/engine/terminal"
)

type WorkspaceReader interface {
	WorktreeDir(
		ctx context.Context,
		workspaceID string,
	) (crowbarHome, projectID, repoID, worktree string, err error)
	// AgentChatsDir returns the directory holding the workspace's agentic chat
	// state — the per-chat handoff ledger and the per-spawn tmp dirs (the rendered
	// hook config; nothing else — no descriptor copies any credential into them,
	// and none may). It is ALWAYS strictly under crowbar
	// home, even for a home-kind / adopted-checkout workspace whose worktree (Cwd)
	// is the user's REAL directory outside home: for a managed worktree it is the
	// sibling of the worktree, and for an adopted checkout it reroots under home
	// so plaintext ledgers never land on the user's filesystem. The worktree/Cwd is
	// unaffected — WorktreeDir still returns it unchanged.
	AgentChatsDir(
		ctx context.Context,
		workspaceID string,
	) (string, error)
}
type ChatLineage interface {
	Ancestors(
		ctx context.Context,
		chatID string,
	) ([]string, error)
}

func (u *Usecase) SpawnChat(
	ctx context.Context,
	workspaceID string,
	providerID string,
) (chatID, runnerID string, err error) {
	chatID = uuid.NewString()
	defer u.spawns.lock(chatID)()

	runnerID, err = u.spawnRunner(ctx, chatID, workspaceID, providerID, "", nil, nil, "", 0, false, "", true, "")
	if err != nil {
		return "", "", u.discardSpawnedChat(ctx, chatID, err)
	}
	return chatID, runnerID, nil
}

func (u *Usecase) MintChat(
	ctx context.Context,
	workspaceID string,
) (string, error) {
	chatID := uuid.NewString()
	created, err := u.chats.Create(ctx, agentchat.CreateInput{
		ID:          chatID,
		WorkspaceID: workspaceID,
		Now:         time.Now(),
	})
	if err != nil {
		return "", fmt.Errorf("agent: mint chat: %w", err)
	}
	u.work.set(chatID, created.Working)
	return chatID, nil
}

func (u *Usecase) StartRunner(
	ctx context.Context,
	chatID string,
	providerID string,
) (string, error) {
	defer u.spawns.lock(chatID)()

	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: start runner: chat: %w", err)
	}
	return u.spawnRunner(ctx, chatID, chat.WorkspaceID, providerID, "", nil, nil, "", 0, false, "", false, "")
}

func (u *Usecase) discardSpawnedChat(
	ctx context.Context,
	chatID string,
	cause error,
) error {
	if err := u.purgeChatLocked(ctx, chatID); err != nil && !errors.Is(err, agentchat.ErrNotFound) {
		slog.WarnContext(ctx, "agent: discard chat of a refused spawn (best-effort, reporting the spawn failure)",
			"chat_id", chatID, "err", err)
	}
	return cause
}

func (u *Usecase) retireChatRunners(
	ctx context.Context,
	chatID string,
) {
	placed, err := u.runners.LiveRunnersForChat(ctx, chatID)
	if err != nil {
		slog.WarnContext(ctx, "agent: look up runners on chat (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return
	}
	for _, r := range placed {
		u.retire(ctx, r)
	}
}

func (u *Usecase) retire(
	ctx context.Context,
	runner domain.AgentRunner,
) {
	if err := u.displace(ctx, runner); err != nil {
		// Best-effort: the runner is still on its chat, so its own exit will close any turn
		// it leaves open. We still kill it.
		slog.ErrorContext(ctx, "agent: retire runner: displace (best-effort, continuing)",
			"runner_id", runner.ID, "chat_id", runner.CurrentChatID, "err", err)
	}
	if err := u.term.TerminateGraceful(ctx, runner.TerminalSession); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: retire runner: terminate (best-effort, continuing)",
			"runner_id", runner.ID, "terminal_session_id", runner.TerminalSession, "err", err)
	}
}

func (u *Usecase) reapCrashOrphanRunnerTmp(
	ctx context.Context,
	runner domain.AgentRunner,
) {
	chatsDir, err := u.ws.AgentChatsDir(ctx, runner.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: chats dir (best-effort, continuing)",
			"runner_id", runner.ID, "workspace_id", runner.WorkspaceID, "err", err)
		return
	}
	home, _, _, _, err := u.ws.WorktreeDir(ctx, runner.WorkspaceID)
	if err != nil {
		slog.WarnContext(ctx, "agent: boot reconcile: reap runner tmp: home (best-effort, continuing)",
			"runner_id", runner.ID, "workspace_id", runner.WorkspaceID, "err", err)
		return
	}
	RemoveUnderHome(ctx, home, worktreepath.RunnerDir(chatsDir, runner.ID, runner.ProviderID))
}

func deriveTitle(prompt string) string {
	for _, line := range strings.Split(prompt, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 60 {
			return strings.TrimSpace(string(r[:60])) + "…"
		}
		return line
	}
	return ""
}

func (u *Usecase) mintRunnerToken(runnerID string) string {
	if u.minter == nil {
		return ""
	}
	return u.minter.Mint(runnerID)
}

func (u *Usecase) replayStartupHook(
	runnerID string,
	hook pendingRunnerHook,
) {
	replayCtx := context.Background()
	if hook.deliveryID != "" {
		replayCtx = context.WithValue(replayCtx, hookDeliveryContextKey{}, hook.deliveryID)
	}
	if err := u.ingestHookNow(
		replayCtx, runnerID, hook.provider, hook.canonicalEvent, hook.rawPayload,
	); err != nil {
		slog.Error("agent: replay startup hook (best-effort, continuing)",
			"runner_id", runnerID, "event", hook.canonicalEvent, "err", err)
		return
	}
	if hook.deliveryID == "" {
		return
	}
	if err := u.hookDeliveries.complete(
		hook.deliveryDir, hook.deliveryID, hook.deliveryHash, time.Now(),
	); err != nil {
		slog.Error("agent: persist replayed startup hook delivery (effects already committed)",
			"runner_id", runnerID, "delivery_id", hook.deliveryID, "err", err)
	}
}

func newRunnerID(
	preallocated string,
) string {
	if preallocated != "" {
		return preallocated
	}
	return uuid.NewString()
}

type spawnContext struct {
	chatID        string
	workspaceID   string
	providerID    string
	projectID     string
	repoID        string
	runnerID      string
	tmpDir        string
	worktree      string
	crowbarHome   string
	threads       string
	conversation  string
	promptMessage string
	gapTurns      int
	resuming      bool
	selection     engineagents.Selection
}
type spawnPreflight struct {
	mcpOn     bool
	threads   string
	selection engineagents.Selection
}

func (u *Usecase) spawnPreflight(
	ctx context.Context,
	chatID, providerID string,
	create bool,
) (spawnPreflight, error) {
	// This is the ONE seam every vendor CLI is launched through, which makes it the
	// only place a disabled provider can actually be stopped.
	if err := u.requireProviderEnabled(ctx, providerID); err != nil {
		return spawnPreflight{}, err
	}
	// The tool switch is a SEPARATE axis from whether the provider is enabled: a CLI
	// spawned with its tools off still comes up, still fires its hooks and still
	// holds a normal chat.
	mcpOn, err := u.providerMCPEnabled(ctx, providerID)
	if err != nil {
		return spawnPreflight{}, err
	}
	threads, err := u.threadContext(ctx, chatID, create)
	if err != nil {
		return spawnPreflight{}, err
	}
	// The selection is also what gets RECORDED on the runner, so the record and the
	// argv are rendered from ONE read: two reads could disagree across a concurrent
	// change and leave a process whose recorded selection is not the one it runs.
	sel, err := u.chatSelection(ctx, chatID, create)
	if err != nil {
		return spawnPreflight{}, err
	}
	return spawnPreflight{mcpOn: mcpOn, threads: threads, selection: sel}, nil
}

type forkRequest struct {
	runnerID    string
	providerID  string
	workspaceID string
	worktree    string
	crowbarHome string
	tmpDir      string
	argv        []string
	env         []string
}

func (u *Usecase) retireOthersOn(
	ctx context.Context,
	chatID string,
	keepID string,
) {
	placed, err := u.runners.LiveRunnersForChat(ctx, chatID)
	if err != nil {
		slog.ErrorContext(ctx, "agent: look up runners placed on chat (best-effort, continuing)",
			"chat_id", chatID, "err", err)
		return
	}
	u.retireOlderThan(ctx, placed, keepID, "chat", chatID)
}

func (u *Usecase) evictHolderOf(
	ctx context.Context,
	runner domain.AgentRunner,
	sessionID string,
) {
	holders, err := u.runners.LiveRunnersForSession(ctx, runner.WorkspaceID, sessionID)
	if err != nil {
		slog.ErrorContext(ctx, "agent: look up runners holding this conversation (best-effort, continuing)",
			"session_id", sessionID, "err", err)
		return
	}
	u.retireOlderThan(ctx, holders, runner.ID, "conversation", sessionID)
}

func (u *Usecase) retireOlderThan(
	ctx context.Context,
	present []domain.AgentRunner,
	keepID string,
	whereKind string,
	whereID string,
) {
	me := -1
	for i, r := range present {
		if r.ID == keepID {
			me = i
			break
		}
	}
	if me < 0 {
		return
	}
	for _, older := range present[me+1:] {
		slog.WarnContext(ctx, "agent: another CLI was already here; retiring it",
			whereKind, whereID, "keeping", keepID, "retiring", older.ID)
		u.retire(ctx, older)
	}
}

func (u *Usecase) teardownAfterPersistFailure(
	ctx context.Context,
	chatID, runnerID, termSessID string,
	cause error,
) error {
	if err := u.term.TerminateGraceful(ctx, termSessID); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: spawn runner: teardown after persist failure",
			"chat_id", chatID, "runner_id", runnerID, "terminal_session_id", termSessID, "err", err)
	}
	return cause
}

func RemoveUnderHome(
	ctx context.Context,
	home string,
	target string,
) {
	if !worktreepath.UnderHome(target, home) {
		slog.WarnContext(ctx, "agent: refusing to rm agent path outside crowbar home (skipping)",
			"target", target, "home", home)
		return
	}
	if err := os.RemoveAll(target); err != nil {
		slog.WarnContext(ctx, "agent: reap agent path", "target", target, "err", err)
	}
}

func (u *Usecase) onRunnerExit(home, runnerID, tmpDir string) func() {
	return func() {
		RemoveUnderHome(context.Background(), home, tmpDir)
		// CreateCommand can observe process exit before recordRunner has a
		// terminal-session id to persist. The startup barrier remembers that fact;
		// spawnRunner reconciles it immediately after persistence and ordered hook
		// replay, instead of this callback missing the not-yet-existing row forever.
		if u.pendingHooks.markExited(runnerID) {
			return
		}
		u.reconcileRunnerExit(context.Background(), runnerID)
	}
}

func (u *Usecase) crowbarHookPath(home string) string {
	if v := os.Getenv("CROWBAR_HOOK_BIN"); v != "" {
		return v
	}
	return filepath.Join(home, "bin", "crowbar")
}

func (u *Usecase) ingestHookNow(
	ctx context.Context,
	runnerID string,
	provider string,
	canonicalEvent string,
	rawPayload []byte,
) error {
	if canonicalEvent == "user_prompt" {
		return u.ingestUserPromptInterlocked(ctx, runnerID, provider, canonicalEvent, rawPayload)
	}
	runner, err := u.runners.Get(ctx, runnerID)
	if err != nil {
		if errors.Is(err, agentrunner.ErrNotFound) {
			// A hook from a runner we do not know (or one whose PTY has already died):
			// ignore it. Never resurrect a runner from a hook.
			return nil
		}
		return fmt.Errorf("agent: ingest hook: runner: %w", err)
	}
	return u.ingestResolvedHook(ctx, runner, provider, canonicalEvent, rawPayload)
}

func (u *Usecase) requirePlacement(
	ctx context.Context,
	runner domain.AgentRunner,
	chatID string,
) (bool, error) {
	if chatID == "" {
		// Already displaced (we are killing it, and it has not fallen over yet).
		u.retire(ctx, runner)
		return false, nil
	}
	_, err := u.chats.GetChat(ctx, chatID)
	if err == nil {
		return true, nil
	}
	if !errors.Is(err, agentchat.ErrNotFound) {
		return false, fmt.Errorf("agent: ingest hook: chat: %w", err)
	}
	slog.WarnContext(ctx, "agent: ingest hook: the runner's chat no longer exists; dropping the announcement and retiring the CLI",
		"runner_id", runner.ID, "chat_id", chatID)
	u.retire(ctx, runner)
	return false, nil
}

func (u *Usecase) appendTurn(
	ctx context.Context,
	chat domain.AgentChat,
	providerID string,
	role, text string,
) error {
	return u.appendRunnerTurn(ctx, chat, providerID, "", "", role, text)
}

func (u *Usecase) appendRunnerTurn(
	ctx context.Context,
	chat domain.AgentChat,
	providerID, runnerID, sessionID string,
	role, text string,
) error {
	if err := u.recordTurn(ctx, chat, providerID, runnerID, sessionID, role, text, ""); err != nil {
		return fmt.Errorf("agent: append turn: %w", err)
	}
	return nil
}

func (u *Usecase) NoteThreadLineage(
	ctx context.Context,
	chatID string,
	ancestors []string,
) error {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: chat: %w", err)
	}
	turns, err := u.chatTurns(ctx, chatID)
	if err != nil {
		return fmt.Errorf("agent: note thread lineage: turns: %w", err)
	}
	if len(turns) == 0 {
		return nil
	}
	return u.appendTurn(ctx, chat, lineageNoteProvider, "user", lineageNoteText(ancestors))
}

const lineageNoteProvider = "crowbar"

func lineageNoteText(
	ancestors []string,
) string {
	return "[Crowbar] This chat was moved in the Chats panel and is a THREAD of " +
		strings.Join(ancestors, ", ") + " (nearest parent first) from this point on. " +
		"Read those chats with get_chat_log. Everything above this line was said BEFORE the move, " +
		"without any of that context: the move changes what this chat reads from now on and " +
		"rewrites nothing it has already read."
}

func (u *Usecase) AssembleHandoff(
	ctx context.Context,
	chatID string,
) (string, error) {
	return u.assembleConversation(ctx, chatID, false, time.Time{})
}

func (u *Usecase) seedWorkFromProjection(
	ctx context.Context,
	chatID string,
) (bool, <-chan struct{}, error) {
	chat, err := u.chats.GetChat(ctx, chatID)
	if err != nil {
		return false, nil, fmt.Errorf("agent: switch provider: inspect chat work: %w", err)
	}
	current, known, changed := u.work.observe(chatID)
	if known {
		return current, changed, nil
	}
	return chat.Working, changed, nil
}

func (u *Usecase) threadContext(
	ctx context.Context,
	chatID string,
	minting bool,
) (string, error) {
	if minting || u.lineage == nil {
		return "", nil
	}
	ancestors, err := u.lineage.Ancestors(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: spawn runner: chat lineage: %w", err)
	}
	if len(ancestors) == 0 {
		return "", nil
	}
	return strings.ReplaceAll(config.GetPrompts().ThreadLineage, "{lineage}", renderLineage(ancestors)), nil
}

func renderLineage(
	ancestors []string,
) string {
	lines := make([]string, 0, len(ancestors))
	for _, id := range ancestors {
		lines = append(lines, "- "+id)
	}
	return strings.Join(lines, "\n")
}

func (u *Usecase) providerMCPEnabled(
	ctx context.Context,
	providerID string,
) (bool, error) {
	pref, err := u.providerPrefs.FindByKey(ctx, providerID)
	if err != nil {
		return false, fmt.Errorf("agent: provider mcp preference %q: %w", providerID, err)
	}
	return pref == nil || !pref.MCPDisabled, nil
}

func (u *Usecase) ListChatsByWorkspace(
	ctx context.Context,
	workspaceID string,
) ([]domain.AgentChat, error) {
	return u.chats.ListByWorkspace(ctx, workspaceID)
}

func (u *Usecase) LiveRunnerForChat(
	ctx context.Context,
	chatID string,
) (domain.AgentRunner, error) {
	return u.runners.LiveRunnerForChat(ctx, chatID)
}

func (u *Usecase) ConversationsForChat(
	ctx context.Context,
	chatID string,
) ([]domain.ChatConversation, error) {
	return u.runners.ConversationsForChat(ctx, chatID)
}
