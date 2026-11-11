package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// WorkspaceBranchRenamer is the narrow write port set_branch_name needs from the
// worktree usecase: renaming the current workspace's branch.
type WorkspaceBranchRenamer interface {
	RenameBranch(
		ctx context.Context,
		wsID string,
		newBranch string,
	) (domain.Workspace, error)
}

// BranchOuter records that the calling thread becomes a branch at the end of its turn.
type BranchOuter interface {
	RequestBranchOut(ctx context.Context, chatID, workspaceID, branch, by string) (bool, error)
}

// branchOutTools is branch_out: deferred to the turn's end, so the agent finishes
// its answer where it is; a repeat while one is pending is the same request.
func branchOutTools(deps Deps) []toolDef {
	if deps.Branches == nil {
		return nil
	}
	return []toolDef{{
		name: "branch_out",
		description: "Make this conversation its own git branch. When your turn ends, Crowbar cuts a new branch " +
			"from this folder's current commit (uncommitted edits are not carried over) and continues this " +
			"conversation in the new worktree. Commit first what should come along. Name the branch for the " +
			"work, short and descriptive (e.g. fix-login-redirect).",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{"name":{"type":"string","description":"Short descriptive branch name; omit to have one generated, and rename it later with set_branch_name."}},
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			var in struct {
				Name string `json:"name"`
			}
			if err := decode(args, &in); err != nil {
				return "", err
			}
			if c.Workspace.RepoID == "" {
				return "", fmt.Errorf("agenttools: branch_out: this folder is not a repository")
			}
			name := strings.TrimSpace(in.Name)
			if _, err := deps.Branches.RequestBranchOut(ctx, c.ChatID, c.Workspace.ID, name, "agent"); err != nil {
				return "", fmt.Errorf("agenttools: branch_out: %w", err)
			}
			if name == "" {
				return "Scheduled: when this turn ends, this conversation continues on a new branch with a " +
					"generated name. Rename it with set_branch_name to a short descriptive one.", nil
			}
			return fmt.Sprintf("Scheduled: when this turn ends, this conversation continues on a new branch %q.", name), nil
		},
	}}
}

func branchNameTools(deps Deps) []toolDef {
	if deps.Workspaces == nil {
		return nil
	}
	return []toolDef{{
		name:        "set_branch_name",
		description: "Rename this workspace's branch. Call once the task is achieved — the branch name should describe what shipped, not what was asked.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{"name":{"type":"string","description":"New branch name."}},
			"required":["name"],
			"additionalProperties":false
		}`),
		run: func(ctx context.Context, c Caller, args json.RawMessage) (string, error) {
			var in struct {
				Name string `json:"name"`
			}
			if err := decode(args, &in); err != nil {
				return "", err
			}
			name := strings.TrimSpace(in.Name)
			if name == "" {
				return "", fmt.Errorf("agenttools: set_branch_name: name must not be empty")
			}
			_, err := deps.Workspaces.RenameBranch(ctx, c.Workspace.ID, name)
			if err != nil {
				return "", fmt.Errorf("agenttools: set_branch_name: %w", err)
			}
			return "Branch renamed to: " + name, nil
		},
	}}
}
