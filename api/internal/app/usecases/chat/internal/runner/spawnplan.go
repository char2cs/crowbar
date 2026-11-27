package runner

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/core/hookserver"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/ledger"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

type spawnInput struct {
	runnerID      string
	launch        *sessionLaunch
	finalSteps    []engineagents.InjectStep
	conversation  string
	gapTurns      int
	resuming      bool
	promptMessage string
	moveNote      string
	selection     *domain.ChatSelection
}

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

func (rs *Runners) bindLaunch(ctx context.Context, chatID string, launch sessionLaunch, handsRecord bool) error {
	var rows []ledger.Row
	if launch.assign != nil {
		rows = append(rows, *launch.assign)
	}
	if launch.afterLoss && handsRecord {
		marker, err := ledger.NewRow(ledger.KindMarker, "", "", ledger.Marker{Kind: ledger.MarkerFromRecord})
		if err != nil {
			return fmt.Errorf("agent: spawn runner: from-record marker: %w", err)
		}
		rows = append(rows, marker)
	}
	if len(rows) == 0 {
		return nil
	}
	if _, _, err := rs.ledger.Append(ctx, chatID, rows...); err != nil {
		return fmt.Errorf("agent: spawn runner: bind launch: %w", err)
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

	chatsDir, err := rs.ws.AgentChatsDir(ctx, cwdWorkspaceID)
	if err != nil {
		return spawnPaths{}, fmt.Errorf("agent: spawn runner: chats dir: %w", err)
	}

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
		worktree:    canonicalDir(worktree),
		tmpDir:      tmpDir,
		chatsDir:    chatsDir,
	}, nil
}

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

func canonicalDir(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		return resolved
	}
	return dir
}

func withPWD(env []string, dir string) []string {
	out := slices.DeleteFunc(slices.Clone(env), func(kv string) bool { return strings.HasPrefix(kv, "PWD=") })
	return append(out, "PWD="+dir)
}

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
		Tmp:             in.tmpDir,
		Cwd:             canonicalDir(in.worktree),
		CrowbarBin:      rs.crowbarBin,
		CrowbarHome:     in.crowbarHome,
		AttachmentsDir:  worktreepath.AttachmentsDir(in.crowbarHome, in.chatID),
		DaemonHost:      rs.daemonHost,
		Segid:           in.runnerID,
		RunnerToken:     rs.mintRunnerToken(in.runnerID),
		RelayID:         in.relayID,
		RelayDir:        worktreepath.RelayDir(in.crowbarHome, in.chatID),
		RelayToken:      rs.mintRelayToken(in.relayID),
		HookBase:        hookBase(in.crowbarHome),
		Provider:        in.providerID,
		ProjectID:       in.projectID,
		RepoID:          in.repoID,
		WorkspaceID:     in.workspaceID,
		ChatID:          in.chatID,
		GapTurns:        strconv.Itoa(in.gapTurns),
		Message:         in.promptMessage,
		Model:           in.selection.Model,
		Effort:          in.selection.Effort,
		PermissionVars:  in.permissionVars,
		ModelParts:      in.modelParts,
		PermissionLevel: in.selection.PermissionLevel,
		Session:         in.launchSessionID,
	}
	tctx.ContextPointer = engineagents.Expand(config.GetPrompts().HandoffPointer, tctx)
	tctx.Context = composeContext(
		engineagents.Expand(config.GetPrompts().CapabilitiesInstruction, tctx),
		in.threads,
		in.conversation,
	)
	inject := in.conversation != "" || !in.resuming
	return tctx, inject
}
