package main

import (
	"context"
	"fmt"
)

const (
	seedFeatureBranch = "feature/pricing-rounding"
	statusDeleted     = "deleted"
)

type chatWorktreeDTO struct {
	Branch    string `json:"branch"`
	Status    string `json:"status"`
	IsDefault bool   `json:"isDefault"`
	LocalPath string `json:"localPath"`
}

type chatDTO struct {
	ID          string           `json:"id"`
	WorkspaceID string           `json:"workspaceId"`
	Worktree    *chatWorktreeDTO `json:"worktree"`
}

type mutationDTO struct {
	ID string `json:"id"`
}

type providerDTO struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

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
