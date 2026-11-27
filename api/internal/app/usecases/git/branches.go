package git

import (
	"context"
	"time"
)

func (u *gitUsecase) CreateBranch(
	ctx context.Context,
	wsID string,
	name string,
	source string,
	switchTo bool,
	now time.Time,
) error {
	if err := validateOperand("name", name); err != nil {
		return err
	}
	if err := validateOperand("source", source); err != nil {
		return err
	}
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.CreateBranch(ctx, repoPath, name, source, switchTo)
	})
}

func (u *gitUsecase) RenameBranch(
	ctx context.Context,
	wsID string,
	oldName string,
	newName string,
	now time.Time,
) error {
	if err := validateOperand("oldName", oldName); err != nil {
		return err
	}
	if err := validateOperand("newName", newName); err != nil {
		return err
	}
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.RenameBranch(ctx, repoPath, oldName, newName)
	})
}

func (u *gitUsecase) DeleteBranch(
	ctx context.Context,
	wsID string,
	name string,
	now time.Time,
) error {
	if err := validateOperand("name", name); err != nil {
		return err
	}
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.DeleteBranch(ctx, repoPath, name)
	})
}

func (u *gitUsecase) SwitchBranch(
	ctx context.Context,
	wsID string,
	name string,
	now time.Time,
) error {
	if err := validateOperand("name", name); err != nil {
		return err
	}
	return u.mutate(ctx, wsID, now, func(repoPath string) error {
		return u.git.SwitchBranch(ctx, repoPath, name)
	})
}
