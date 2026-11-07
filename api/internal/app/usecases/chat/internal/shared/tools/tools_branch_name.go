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
			"conversation in the new worktree, once the person allows it. Commit first what should come along.",
		schema: json.RawMessage(`{
			"type":"object",
			"properties":{"name":{"type":"string","description":"Branch name; omit to have one generated."}},
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
			if _, err := deps.Branches.RequestBranchOut(ctx, c.ChatID, c.Workspace.ID, strings.TrimSpace(in.Name), "agent"); err != nil {
				return "", fmt.Errorf("agenttools: branch_out: %w", err)
			}
			return "Crowbar asked the person to allow this branch. If they do, when this turn ends this " +
				"conversation continues on the new branch; until then keep working here.", nil
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
