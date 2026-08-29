package runner

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"github.com/char2cs/crowbar/api/internal/core/config"
	"github.com/char2cs/crowbar/api/internal/core/paths/worktreepath"
	engineagents "github.com/char2cs/crowbar/api/internal/engine/agents"
)

// Planning a spawn: where it runs, what context it is handed, and whether it may
// start at all.
//
// Everything here is decided BEFORE a process exists, which is what makes it
// separable from the fork that follows. A plan that is wrong costs nothing; a fork
// that is wrong costs a CLI on the user's machine.

type spawnPaths struct {
	crowbarHome string
	projectID   string
	repoID      string
	worktree    string
	tmpDir      string
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

	// Under the workspace's chats dir (always beneath crowbar home), keyed by the RUNNER —
	// id + provider — and NOT by the chat. The chat pointer is erasable (Displace clears it
	// while the process still runs), and a dir that can only be found through an erasable
	// pointer can never be reaped; see worktreepath.RunnerDir. This path is derivable from
	// a bare runner row forever, which is what lets BOTH removers find it: onExit below on a
	// clean death, and boot reconciliation when the daemon died before onExit could run.
	//
	// It holds the rendered hook config the CLI is pointed at, and must survive for the
	// whole life of that CLI — so it is removed on PTY death, never eagerly after spawn.
	//
	// It holds no secret: a provider owns its own credentials and Crowbar never copies
	// them anywhere (that rule is why CODEX_HOME is not ours, and why codex's sessions
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

func (rs *Runners) renderSpawnContext(
	in spawnContext,
) (engineagents.TemplateCtx, bool) {
	tctx := engineagents.TemplateCtx{
		Tmp:         in.tmpDir,
		Cwd:         in.worktree,
		CrowbarHook: rs.crowbarHookPath(in.crowbarHome),
		Segid:       in.runnerID,
		// The credential the descriptors hand `crowbar mcp` so this runner's tool
		// calls can be attributed to it. Minted here because this is where the
		// runner id is born, and by the SAME minter DispatchMCP verifies against —
		// a token minted anywhere else would authenticate nothing.
		RunnerToken: rs.mintRunnerToken(in.runnerID),
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
		// Templated against by an api-transport descriptor's serve/attach argv
		// ({socket}). A short path under the OS temp dir, never under the
		// worktree — macOS's sun_path is a hard 104 bytes (see
		// [[project_dev_home_isolation]]).
		Socket: apiSocketPath(in.runnerID),
		// The prior session id, if any — applyAPITransport overwrites this with
		// whatever EstablishSession actually confirmed (a resumed id unchanged,
		// or a freshly minted one) before attach's argv is ever rendered.
		Session: in.launchSessionID,
	}
	// The message for a provider that can ONLY be reached through a user message (a
	// resumed codex ignores every config channel). It POINTS at the ledger already on
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
	// resume's ResumeContextInject can be a USER MESSAGE — a resumed codex can be
	// reached no other way (see codex.yaml) — and reopening a closed tab resumes a
	// provider with nothing recorded in between. Letting the preamble deliver there
	// would open every revived codex chat with a "while you were away" pointer about
	// nothing, and codex answers its opening message on sight.
	//
	// The cost is that a provider whose resume channel IS silent (claude's is a flag)
	// also loses the preamble on a gapless revive. That is deliberate over the
	// alternative: whether a resume channel speaks out loud is the DESCRIPTOR's
	// knowledge, and inventing a "this channel is silent" field to recover one
	// directive would put a provider's manners in this package. Its tools are still
	// registered on that spawn either way — only the directive is missing.
	inject := in.conversation != "" || !in.resuming
	return tctx, inject
}
