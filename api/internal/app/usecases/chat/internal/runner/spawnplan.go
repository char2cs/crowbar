package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Planning a spawn: where it runs, what context it is handed, and whether it may
// start at all.
//
// Everything here is decided BEFORE a process exists, which is what makes it
// separable from the fork that follows. A plan that is wrong costs nothing; a fork
// that is wrong costs a CLI on the user's machine.

// spawnInput is what one spawn launches with beyond its chat and provider.
type spawnInput struct {
	runnerID string // preallocated; "" mints one
	// launch is the session decision the caller already made; nil makes it here.
	launch        *sessionLaunch
	finalSteps    []engineagents.InjectStep
	conversation  string
	gapTurns      int
	resuming      bool // the content signal: is the handoff a gap or the whole conversation
	promptMessage string
	// selection is a pick staged with the prompt, launched in place of the
	// chat's own and made the chat's once the launch runs; nil stages none.
	selection *domain.ChatSelection
}

// spawnLaunch is the spawn's session decision and the argv steps carrying it.
func (rs *Runners) spawnLaunch(
	ctx context.Context, chatID string, d engineagents.Agent, planned *sessionLaunch,
) (sessionLaunch, []engineagents.InjectStep, error) {
	if planned == nil {
		launch, err := rs.planLaunch(ctx, chatID, d)
		if err != nil {
			return sessionLaunch{}, nil, err
		}
		planned = &launch
	}
	steps, err := planned.steps(d)
	return *planned, steps, err
}

// bindAssigned appends the binding of a minted session before the CLI that
// will announce it exists, so the announcement finds it already bound.
func (rs *Runners) bindAssigned(ctx context.Context, chatID string, launch sessionLaunch) error {
	if launch.assign == nil {
		return nil
	}
	if _, _, err := rs.ledger.Append(ctx, chatID, *launch.assign); err != nil {
		return fmt.Errorf("agent: spawn runner: bind assigned session: %w", err)
	}
	return nil
}

type spawnPaths struct {
	crowbarHome string
	projectID   string
	repoID      string
	worktree    string
	tmpDir      string
	chatsDir    string
}

func (rs *Runners) spawnPaths(
	ctx context.Context,
	chatID string,
	workspaceID string,
	runnerID string,
	providerID string,
) (spawnPaths, error) {
	cwdWorkspaceID, err := rs.cwdWorkspaceID(ctx, chatID, workspaceID)
	if err != nil {
		return spawnPaths{}, err
	}
	crowbarHome, projectID, repoID, worktree, err := rs.ws.WorktreeDir(ctx, cwdWorkspaceID)
	if err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: worktree dir: %w", err)
	}

	// The chats dir is resolved separately from the worktree/Cwd: for a home-kind
	// (adopted checkout) workspace the worktree is the user's REAL dir outside home,
	// so chat state (this tmp dir, the ledger) reroots under crowbar home while the
	// CLI still runs with Cwd = worktree.
	chatsDir, err := rs.ws.AgentChatsDir(ctx, cwdWorkspaceID)
	if err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: chats dir: %w", err)
	}

	// Keyed by the runner's id and provider (worktreepath.RunnerDir), so a
	// runner_started row alone finds it: the boot reaps what onExit never ran for.
	//
	// It holds the rendered hook config the CLI is pointed at, and must survive for the
	// whole life of that CLI — so it is removed on PTY death, never eagerly after spawn.
	//
	// It holds no secret: a provider owns its own credentials and Crowbar never copies
	// them anywhere (that rule is why a provider's home is not ours, and why its sessions
	// survive a switch). Nothing in the descriptors puts a credential here, and nothing
	// may.
	tmpDir := worktreepath.RunnerDir(chatsDir, runnerID, providerID)
	if err := os.MkdirAll(tmpDir, 0o700); err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: mkdir tmp: %w", err)
	}
	return spawnPaths{
		crowbarHome: crowbarHome,
		projectID:   projectID,
		repoID:      repoID,
		worktree:    worktree,
		tmpDir:      tmpDir,
		chatsDir:    chatsDir,
	}, nil
}

// cwdWorkspaceID answers the workspace a cwd-dependent op resolves
// WorktreeDir/AgentChatsDir against: workspaceID itself for an ordinary
// workspace-owning chat, or its nearest workspace-owning ancestor's for a
// bubble (chat.WorkspaceID == "") — ancestorCwd's own answer to where a row's
// CLI runs (model spec: a bubble under a worktree-owning row runs in that
// row's worktree).
//
// Shared by every caller that needs a real workspace to run a bubble's CLI
// against, not only spawnPaths: promptTarget, Compact and SlashCatalog all
// resolve a LIVE runner's own worktree the same way.
func (rs *Runners) cwdWorkspaceID(
	ctx context.Context,
	chatID string,
	workspaceID string,
) (string, error) {
	if workspaceID != "" {
		return workspaceID, nil
	}
	ancestor, ok, err := rs.ancestorCwd.ResolveCwdWorkspaceID(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: resolve cwd workspace: %w", err)
	}
	if !ok {
		return "", fmt.Errorf("agent: resolve cwd workspace: chat %s has no workspace-owning ancestor to run in", chatID)
	}
	return ancestor, nil
}

// canonicalDir is dir with symlinks resolved, or dir itself when it cannot be.
// A provider keys its folder trust on the canonical path, so a symlinked
// worktree would otherwise park the CLI on a trust prompt.
func canonicalDir(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return dir
}

// hookBase is where the daemon published its hook server under home. Without one a
// CLI's hooks have nowhere to go, which is loud here because it is silent there.
func hookBase(
	home string,
) string {
	base, err := hookserver.Read(home)
	if err != nil {
		slog.Error("agent: the hook server is not published; this CLI's hooks will not reach the daemon",
			"home", home, "err", err)
		return ""
	}
	return base
}

func (rs *Runners) renderSpawnContext(
	in spawnContext,
) (engineagents.TemplateCtx, bool) {
	tctx := engineagents.TemplateCtx{
		Tmp:          in.tmpDir,
		Cwd:          canonicalDir(in.worktree),
		CrowbarHook:  rs.crowbarHookPath(in.crowbarHome),
		CrowbarHome:  in.crowbarHome,
		DaemonSocket: rs.daemonSocket,
		Segid:        in.runnerID,
		// The credential the descriptors hand `crowbar mcp` so this runner's tool
		// calls can be attributed to it. Minted here because this is where the
		// runner id is born, and by the SAME minter DispatchMCP verifies against —
		// a token minted anywhere else would authenticate nothing.
		RunnerToken: rs.mintRunnerToken(in.runnerID),
		HookBase:    hookBase(in.crowbarHome),
		Provider:    in.providerID,
		ProjectID:   in.projectID,
		RepoID:      in.repoID,
		WorkspaceID: in.workspaceID,
		ChatID:      in.chatID,
		GapTurns:    strconv.Itoa(in.gapTurns),
		Message:     in.promptMessage,
		// Read by the descriptor's own model:/effort: apply steps and by nothing
		// else. They are empty on a chat that has chosen nothing, and the steps
		// that would read them are not rendered at all in that case.
		Model:  in.selection.Model,
		Effort: in.selection.Effort,
		// The resolved permission level's own named values, already looked
		// up by the caller against the SAME descriptor SpawnPlan renders
		// against — see spawn.go's own comment on why the lookup happens
		// there, not here.
		PermissionVars: in.permissionVars,
		// The prior session id, if any — applyAPITransport overwrites this with
		// whatever EstablishSession actually confirmed (a resumed id unchanged,
		// or a freshly minted one) before attach's argv is ever rendered.
		Session: in.launchSessionID,
	}
	// The message for a provider that can ONLY be reached through a user message (one
	// that, resumed, ignores every config channel). It POINTS at the ledger already on
	// disk and says where to start reading — it never carries the transcript, which
	// would dump the whole handed-off exchange into the chat for the user to scroll
	// past. An agent reads files.
	tctx.ContextPointer = engineagents.Expand(config.GetPrompts().HandoffPointer, tctx)
	// The preamble first (which tools exist), then the lineage (which OTHER chats
	// this one reads), then the handoff (what was said here already). Orientation
	// before history: a model should know it has get_chat_log and which ids to
	// point it at before it reads a conversation it is told not to act on.
	tctx.Context = composeContext(
		engineagents.Expand(config.GetPrompts().CapabilitiesInstruction, tctx),
		in.threads,
		in.conversation,
	)
	// WHETHER to deliver that document at all, which is a separate question from what
	// it says.
	//
	// A handoff is something that HAPPENED and is always worth delivering. The
	// capability preamble is standing orientation, so it may only drive delivery down a
	// SILENT channel: a fresh spawn's ContextInject is a config key or a flag, but a
	// resume's ResumeContextInject can be a USER MESSAGE — some resumed providers can
	// be reached no other way (their descriptor says so) — and reopening a closed tab
	// resumes a provider with nothing recorded in between. Letting the preamble deliver
	// there would open every such revived chat with a "while you were away" pointer
	// about nothing, which the provider answers on sight.
	//
	// The cost is that a provider whose resume channel IS silent (a flag, say)
	// also loses the preamble on a gapless revive. That is deliberate over the
	// alternative: whether a resume channel speaks out loud is the DESCRIPTOR's
	// knowledge, and inventing a "this channel is silent" field to recover one
	// directive would put a provider's manners in this package. Its tools are still
	// registered on that spawn either way — only the directive is missing.
	inject := in.conversation != "" || !in.resuming
	return tctx, inject
}
