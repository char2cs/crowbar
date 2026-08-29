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
