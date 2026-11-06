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

// Planning a spawn — where it runs, what context it gets, whether it may start — all
// decided BEFORE a process exists: a wrong plan costs nothing, a wrong fork costs a CLI.

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

	// The chats dir is resolved apart from the Cwd: an adopted checkout's worktree is
	// the user's REAL dir, so chat state reroots under crowbar home.
	chatsDir, err := rs.ws.AgentChatsDir(ctx, cwdWorkspaceID)
	if err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: chats dir: %w", err)
	}

	// Keyed by runner id and provider so a runner_started row alone finds it for the
	// boot reap. It holds the CLI's hook config for the CLI's whole life (removed on PTY
	// death) and never a credential: a provider owns those, and Crowbar never copies them.
	tmpDir := worktreepath.RunnerDir(chatsDir, runnerID, providerID)
	rs.runnerDirs.Lock()
	err = os.MkdirAll(tmpDir, 0o700)
	rs.runnerDirs.Unlock()
	if err != nil {
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

// cwdWorkspaceID answers the workspace a cwd-dependent op resolves against: workspaceID
// itself, or for a bubble (WorkspaceID == "") its nearest workspace-owning ancestor's,
// whose worktree a bubble runs in.
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
		Tmp:         in.tmpDir,
		Cwd:         canonicalDir(in.worktree),
		CrowbarBin:  rs.crowbarBin,
		CrowbarHome: in.crowbarHome,
		DaemonHost:  rs.daemonHost,
		Segid:       in.runnerID,
		// The token `crowbar mcp` attributes this runner's tool calls by, minted where
		// the runner id is born and by the SAME minter DispatchMCP verifies against.
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
		// The resolved permission level's named values, looked up by the caller
		// against the SAME descriptor SpawnPlan renders.
		PermissionVars:  in.permissionVars,
		PermissionLevel: in.selection.PermissionLevel,
		// The prior session id, if any — applyAPITransport overwrites this with
		// whatever EstablishSession actually confirmed (a resumed id unchanged,
		// or a freshly minted one) before attach's argv is ever rendered.
		Session: in.launchSessionID,
	}
	// The message for a provider reachable ONLY through a user message: it POINTS at the
	// ledger on disk rather than carrying the transcript into the chat. An agent reads files.
	tctx.ContextPointer = engineagents.Expand(config.GetPrompts().HandoffPointer, tctx)
	// Preamble (tools), then lineage (which OTHER chats it reads), then handoff:
	// orientation before history.
	tctx.Context = composeContext(
		engineagents.Expand(config.GetPrompts().CapabilitiesInstruction, tctx),
		in.threads,
		in.conversation,
	)
	// Deliver a handoff always, but the preamble alone only on a fresh spawn: a resume's
	// inject may be a USER MESSAGE the provider answers on sight. A silent resume channel
	// thus also loses the preamble on a gapless revive; its tools are still registered.
	inject := in.conversation != "" || !in.resuming
	return tctx, inject
}
