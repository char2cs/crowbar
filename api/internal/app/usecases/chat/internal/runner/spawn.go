package runner

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"

	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/promptsigil"
	"github.com/char2cs/crowbar/api/internal/app/usecases/chat/internal/shared/snapshot"
	engineterminal "github.com/char2cs/crowbar/api/internal/core/terminal"
	"github.com/char2cs/crowbar/api/internal/domain"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

func (rs *Runners) StartRunner(ctx context.Context, chatID, providerID string) (string, error) {
	return rs.underChat(ctx, chatID, "start runner", func(chat domain.Chat) (string, error) {
		return rs.startLocked(ctx, chat, providerID)
	})
}

func (rs *Runners) OpenChat(ctx context.Context, chatID, providerID string) (string, error) {
	return rs.underChat(ctx, chatID, "open chat", func(chat domain.Chat) (string, error) {
		return rs.openLocked(ctx, chat, providerID)
	})
}

func (rs *Runners) underChat(
	ctx context.Context, chatID, op string, launch func(domain.Chat) (string, error),
) (string, error) {
	h, err := rs.acquire(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: %s: %w", op, err)
	}
	defer h.release()
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return "", fmt.Errorf("agent: %s: chat: %w", op, err)
	}
	return launch(chat)
}

func (rs *Runners) openLocked(ctx context.Context, chat domain.Chat, providerID string) (string, error) {
	if err := rs.providers.RequireProviderEnabled(ctx, providerID); err != nil {
		return "", err
	}
	descriptor, _, err := rs.promptAgent(ctx, chat, providerID)
	if err != nil {
		return "", err
	}
	if surfaceForSpawn(descriptor, chat.Surface) != engineagents.SurfaceTerminal {
		return "", nil
	}
	return rs.startLocked(ctx, chat, providerID)
}

func (rs *Runners) startLocked(ctx context.Context, chat domain.Chat, providerID string) (string, error) {
	defer rs.enterPhase(ctx, chat.ID, snapshot.PhaseStarting)()
	return rs.spawnRunner(ctx, chat.ID, chat.WorkspaceID, providerID, spawnInput{})
}

//nolint:funlen // orchestrates preflight, path resolution, attachment materialization, descriptor render, spawn-plan build and fork in one strict sequence; splitting would scatter the abort-on-failure cleanup this function is responsible for at each step
func (rs *Runners) spawnRunner(
	ctx context.Context,
	chatID string,
	workspaceID string,
	providerID string,
	in spawnInput,
) (_ string, err error) {
	ctx = context.WithoutCancel(ctx)
	runnerID := newRunnerID(in.runnerID)
	defer rs.recordSpawnFailure(ctx, chatID, runnerID, &err)
	pre, err := rs.spawnPreflight(ctx, chatID, providerID, in.selection)
	if err != nil {
		return "", err
	}
	threads, sel := pre.threads, pre.selection

	paths, err := rs.spawnPaths(ctx, chatID, workspaceID, runnerID, providerID)
	if err != nil {
		return "", err
	}
	crowbarHome, projectID, repoID := paths.crowbarHome, paths.projectID, paths.repoID
	worktree, tmpDir := paths.worktree, paths.tmpDir

	descriptor, err := rs.agents.Get(ctx, crowbarHome, providerID)
	if err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", fmt.Errorf("agent: spawn runner: resolve descriptor: %w", err)
	}
	launch, sessionSteps, err := rs.spawnLaunch(ctx, chatID, descriptor, in.launch)
	if err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", err
	}

	sigils, escape := descriptor.PromptLeadingSigils()
	guardedMessage := promptsigil.Guard(sigils, escape, chatID, in.promptMessage)
	dispatchMessage := in.moveNote + materializeAttachmentsForDispatch(paths.crowbarHome, chatID, guardedMessage)
	dispatchConversation := materializeAttachmentsForDispatch(paths.crowbarHome, chatID, in.conversation)

	descriptor = descriptor.WithTools(pre.mcpOn)

	sel = resolveSelectionForSpawn(descriptor, sel)
	staged := pre.staged
	if staged != nil {
		staged = &domain.ChatSelection{Model: sel.Model, Effort: sel.Effort}
	}

	tctx, inject := rs.renderSpawnContext(spawnContext{
		chatID:          chatID,
		workspaceID:     workspaceID,
		providerID:      providerID,
		projectID:       projectID,
		repoID:          repoID,
		runnerID:        runnerID,
		tmpDir:          tmpDir,
		worktree:        worktree,
		crowbarHome:     crowbarHome,
		launchSessionID: launch.resumeID,
		relayID:         launch.relayID,
		threads:         threads,
		conversation:    dispatchConversation,
		promptMessage:   dispatchMessage,
		gapTurns:        in.gapTurns,
		resuming:        in.resuming,
		selection:       sel,
		permissionVars:  descriptor.PermissionVars(sel.PermissionLevel, pre.left),
		modelParts:      descriptor.ModelParts(sel.Model),
	})

	if inject {
		rs.agents.RecordInjection(runnerID, tctx.Context, tctx.ContextPointer)
	}
	if in.moveNote != "" {
		rs.agents.RecordInjection(runnerID, strings.TrimSpace(in.moveNote))
		rs.noteLevelNarrowed(ctx, chatID, descriptor, sel.PermissionLevel, tctx.PermissionVars)
	}

	if err := rs.live.add(engineagents.Runner{
		ID: runnerID, WorkspaceID: workspaceID, ProviderID: providerID, CurrentChatID: chatID,
	}); err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", err
	}
	rs.surfaces.set(runnerID, surfaceForSpawn(descriptor, pre.surface))
	attachArgv, refused := rs.apiTransportForSurface(
		ctx, runnerID, providerID, descriptor, tctx, resumeContextFor(in.resuming, inject, tctx), pre.surface,
	)
	if refused != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		if errors.Is(refused, engineagents.ErrAPISessionLost) {
			return "", fmt.Errorf("%w: %w", ErrSpawnSessionRefused, refused)
		}
		return "", refused
	}
	steps := buildSpawnSteps(
		descriptor, in.resuming, inject, sel, sessionSteps, in.finalSteps,
	)

	plan, err := descriptor.SpawnPlan(tctx, os.Environ(), steps)
	if err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", fmt.Errorf("agent: spawn runner: build spawn plan: %w", err)
	}
	plan.Env = withPWD(plan.Env, tctx.Cwd)
	pointPlanAtAttach(plan, attachArgv)
	if err := rs.bindLaunch(ctx, chatID, launch, in.conversation != ""); err != nil {
		rs.abandonUnforked(ctx, runnerID, paths)
		return "", err
	}

	termSessID, carried, err := rs.forkOrAdopt(ctx, forkRequest{
		runnerID:      runnerID,
		providerID:    providerID,
		chatID:        chatID,
		worktree:      worktree,
		crowbarHome:   crowbarHome,
		tmpDir:        tmpDir,
		argv:          append([]string{plan.Executable}, plan.Argv...),
		env:           plan.Env,
		promptMessage: dispatchMessage,
		argvSelection: carriedSelection(sel, descriptor.SelectionSteps),
		apiSelection:  carriedSelection(sel, descriptor.SelectionAPISteps),
		resume:        resumeWatch{chatID: chatID, providerID: providerID, sessionID: launch.resumeID, agent: descriptor},
	}, attachArgv)
	if err != nil {
		return "", err
	}

	if err := rs.recordRunner(ctx, engineagents.Runner{
		ID: runnerID, WorkspaceID: workspaceID, ProviderID: providerID, TerminalSession: termSessID,
		CurrentChatID: chatID, LaunchModel: carried.Model, LaunchEffort: carried.Effort,
		LaunchPermissionLevel: carried.PermissionLevel,
	}, launch.resumeID, staged); err != nil {
		if !errors.Is(err, ErrProviderExitedDuringStartup) {
			rs.agents.ForgetRunner(runnerID)
		}
		return "", err
	}
	return runnerID, nil
}

func (rs *Runners) abandonUnforked(ctx context.Context, runnerID string, paths spawnPaths) {
	rs.apiConns.drop(runnerID)
	rs.agents.ForgetRunner(runnerID)
	rs.surfaces.drop(runnerID)
	rs.reapRunnerDir(ctx, paths.crowbarHome, paths.tmpDir)
}

func (rs *Runners) forkCLI(
	ctx context.Context,
	req forkRequest,
) (string, error) {
	//nolint:contextcheck // the exit outlives the request; its values must not reach it.
	termSessID, err := rs.term.CreateCommand(ctx, req.chatID, req.worktree, req.argv, req.env,
		rs.onRunnerExit(req.crowbarHome, req.runnerID, req.tmpDir, req.resume))
	if err == nil {
		return termSessID, nil
	}
	rs.abandonUnforked(ctx, req.runnerID, req.paths())

	if errors.Is(err, engineterminal.ErrCommandNotFound) {
		return "", fmt.Errorf("%w: %s", engineterminal.ErrCommandNotFound, req.providerID)
	}
	return "", fmt.Errorf("agent: spawn runner: create command: %w", err)
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
	chatID         string
	workspaceID    string
	providerID     string
	projectID      string
	repoID         string
	runnerID       string
	tmpDir         string
	worktree       string
	crowbarHome    string
	threads        string
	conversation   string
	promptMessage  string
	gapTurns       int
	resuming       bool
	selection      engineagents.Selection
	permissionVars map[string]string
	modelParts     map[string]string

	launchSessionID string
	relayID         string
}

type spawnPreflight struct {
	mcpOn     bool
	threads   string
	selection engineagents.Selection
	staged    *domain.ChatSelection
	surface   string
	left      bool
}

func (rs *Runners) spawnPreflight(
	ctx context.Context,
	chatID, providerID string,
	staged *domain.ChatSelection,
) (spawnPreflight, error) {
	if err := rs.providers.RequireProviderEnabled(ctx, providerID); err != nil {
		return spawnPreflight{}, err
	}
	mcpOn, err := rs.providers.ProviderMCPEnabled(ctx, providerID)
	if err != nil {
		return spawnPreflight{}, err
	}
	threads, err := rs.conversations.ThreadContext(ctx, chatID)
	if err != nil {
		return spawnPreflight{}, err
	}
	sel, err := rs.conversations.ChatSelection(ctx, chatID)
	if err != nil {
		return spawnPreflight{}, err
	}
	chat, err := rs.chats.GetChat(ctx, chatID)
	if err != nil {
		return spawnPreflight{}, fmt.Errorf("agent: spawn runner: chat surface: %w", err)
	}
	if staged, err = rs.movedPick(ctx, chat, providerID, staged); err != nil {
		return spawnPreflight{}, err
	}
	if staged != nil {
		sel.Model, sel.Effort = staged.Model, staged.Effort
	}
	st, err := rs.ledger.State(ctx, chatID)
	if err != nil {
		return spawnPreflight{}, fmt.Errorf("agent: spawn runner: chat state: %w", err)
	}
	return spawnPreflight{
		mcpOn: mcpOn, threads: threads, selection: sel, staged: staged, surface: chat.Surface,
		left: len(st.LeftDirs()) > 0,
	}, nil
}

type forkRequest struct {
	runnerID      string
	providerID    string
	chatID        string
	worktree      string
	crowbarHome   string
	tmpDir        string
	argv          []string
	env           []string
	promptMessage string
	argvSelection engineagents.Selection
	apiSelection  engineagents.Selection
	resume        resumeWatch
}

func (req forkRequest) paths() spawnPaths {
	return spawnPaths{crowbarHome: req.crowbarHome, tmpDir: req.tmpDir}
}

func (rs *Runners) teardownForked(
	ctx context.Context,
	chatID, runnerID, termSessID string,
	cause error,
) error {
	if err := rs.term.TerminateGraceful(ctx, termSessID); err != nil &&
		!errors.Is(err, engineterminal.ErrSessionNotFound) {
		slog.WarnContext(ctx, "agent: spawn runner: teardown of a failed spawn",
			"chat_id", chatID, "runner_id", runnerID, "terminal_session_id", termSessID, "err", err)
	}
	rs.apiConns.drop(runnerID)
	return cause
}
