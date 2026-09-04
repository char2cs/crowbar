package main

import (
	"context"
	"fmt"
)

const (
	seedFeatureBranch = "feature/pricing-rounding"
	statusDeleted     = "deleted"
)

// chatWorktreeDTO is the git half of a chat that owns a worktree — the
// seed's own slice of dto.ChatWorktreeDTO: branch, wire status, whether it is
// the repo's own adopted home, and where it lives on disk.
type chatWorktreeDTO struct {
	Branch    string `json:"branch"`
	Status    string `json:"status"`
	IsDefault bool   `json:"isDefault"`
	LocalPath string `json:"localPath"`
}

// chatDTO is the seed's own slice of dto.AgentChatDTO / AgentChatDetailDTO:
// enough to find, read back and report the worktree a chat owns. Worktree is
// nil for a chat that owns none (a plain conversation, a folder).
type chatDTO struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspaceId"`
	Worktree    *chatWorktreeDTO `json:"worktree"`
}

// mutationDTO is the {id} envelope every v0 mutation answers with.
type mutationDTO struct {
	ID string `json:"id"`
}

// providerDTO is the seed's own slice of dto.AgentProviderDTO: just enough to
// pick a provider to spawn the forked feature chat's runner with.
type providerDTO struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// pickBaseChat finds the locked, Crowbar-managed branch chat sitting on the
// repo's default branch — the branch-tree root a feature chat forks from.
//
// It is deliberately NOT the isDefault "home" row. Import adopts the repo
// folder itself as home and detaches it so the base branch is free for its
// own managed worktree; home is the unmanaged checkout Crowbar never runs git
// on, and a child forked off it is parented to a row that owns no branch.
//
// GET .../repos/:repoId/chats is already scoped to this repo server-side
// (ListChatsInRepo's cwd walk resolves each row's owning workspace and
// compares its repo against the URL), so — unlike the deleted workspace list,
// which answered with every repo's rows — there is no repo id left for this
// picker to filter by.
func pickBaseChat(
	defaultBranch string,
) func([]chatDTO) (chatDTO, bool) {
	return func(list []chatDTO) (chatDTO, bool) {
		for _, c := range list {
			wt := c.Worktree
			if wt != nil && wt.Branch == defaultBranch && !wt.IsDefault && wt.Status != statusDeleted {
				return c, true
			}
		}
		return chatDTO{}, false
	}
}

// pickFeatureChat requires a populated LocalPath: the chat's worktree is what
// the seed then commits into, and a row without one on disk is not yet
// usable for that.
func pickFeatureChat(
	list []chatDTO,
) (chatDTO, bool) {
	for _, c := range list {
		wt := c.Worktree
		if wt != nil && wt.Branch == seedFeatureBranch && wt.Status != statusDeleted && wt.LocalPath != "" {
			return c, true
		}
	}
	return chatDTO{}, false
}

// waitForBaseChat blocks until repo import has provisioned the locked base
// worktree's owning chat. It is provisioned after the repo row is broadcast,
// so a fork issued the instant the repo appears would have no chat to parent
// itself under.
func waitForBaseChat(
	ctx context.Context,
	d *daemon,
	repo repoDTO,
) (chatDTO, error) {
	return waitFor(
		ctx, d,
		"the locked "+repo.DefaultBranch+" workspace",
		chatsPath(repo.ProjectID, repo.ID),
		pickBaseChat(repo.DefaultBranch),
	)
}

// resolveProvider picks a connected provider to spawn the forked feature
// chat's runner with. A fork always starts one — own_worktree.go's
// SpawnChatWithOwnWorktree has no empty-providerID skip the import path
// gives itself — so unlike the rest of this tool, forking cannot run with
// nothing configured.
func resolveProvider(
	ctx context.Context,
	d *daemon,
	repo repoDTO,
) (string, error) {
	providers, err := getData[[]providerDTO](ctx, d, "list providers", chatsPath(repo.ProjectID, repo.ID)+"/providers")
	if err != nil {
		return "", err
	}
	for _, p := range providers {
		if p.Enabled {
			return p.ID, nil
		}
	}
	return "", fmt.Errorf(
		"seed: no enabled provider to fork the %s workspace with — connect one and re-run", seedFeatureBranch,
	)
}

// ensureFeatureChat creates the seed's feature chat forked from the base
// chat, or reuses the one already there.
//
// A fork always lands with a server-generated branch name (model spec §4.1)
// rather than the one asked for, and always starts the forking provider's
// CLI — so this stops the runner again immediately (the seed wants the
// workspace, not a live conversation left running after it exits) and
// renames the branch back to the seed's own fixed name.
func ensureFeatureChat(
	ctx context.Context,
	d *daemon,
	repo repoDTO,
	baseChatID string,
) (chatDTO, bool, error) {
	path := chatsPath(repo.ProjectID, repo.ID)
	existing, err := getData[[]chatDTO](ctx, d, "list chats", path)
	if err != nil {
		return chatDTO{}, false, err
	}
	if found, ok := pickFeatureChat(existing); ok {
		return found, false, nil
	}

	provider, err := resolveProvider(ctx, d, repo)
	if err != nil {
		return chatDTO{}, false, err
	}
	body := map[string]any{"provider": provider, "parentId": baseChatID, "ownWorktree": true}
	created, err := postData[mutationDTO](ctx, d, "fork the "+seedFeatureBranch+" workspace", path, body)
	if err != nil {
		return chatDTO{}, false, err
	}

	detail := chatDetailPath(repo.ProjectID, repo.ID, created.ID)
	if err := d.postAccepted(ctx, "stop the forked chat's runner", detail+"/stop", nil); err != nil {
		return chatDTO{}, false, err
	}
	renameBody := map[string]any{"branch": seedFeatureBranch}
	if _, err := patchData[mutationDTO](ctx, d, "name the forked branch", detail+"/branch", renameBody); err != nil {
		return chatDTO{}, false, err
	}

	pickCreated := func(list []chatDTO) (chatDTO, bool) {
		for _, c := range list {
			if c.ID == created.ID && c.Worktree != nil && c.Worktree.Branch == seedFeatureBranch {
				return c, true
			}
		}
		return chatDTO{}, false
	}
	chat, err := waitFor(ctx, d, "the renamed "+seedFeatureBranch+" workspace", path, pickCreated)
	return chat, true, err
}
