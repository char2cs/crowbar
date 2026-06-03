package usecases

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/repositories"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// GitUsecase is the public contract for git operations on a task's worktree.
type GitUsecase interface {
	Log(
		ctx context.Context,
		taskID string,
	) ([]domain.GitCommit, error)

	Diff(
		ctx context.Context,
		taskID string,
	) (string, error)

	ListFiles(
		ctx context.Context,
		taskID string,
	) ([]string, error)
}

type gitUsecase struct {
	taskRepo repositories.Task
}

// NewGit wires a Task repository into a GitUsecase.
func NewGit(
	taskRepo repositories.Task,
) GitUsecase {
	return &gitUsecase{taskRepo: taskRepo}
}

func (u *gitUsecase) Log(
	ctx context.Context,
	taskID string,
) ([]domain.GitCommit, error) {
	task, err := u.taskRepo.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}

	out, err := runGitCmd(ctx, task.WorktreePath, "log", "--format=%H|%s|%an|%ai", "HEAD")
	if err != nil {
		return nil, fmt.Errorf("git usecase: log: %w", err)
	}

	var commits []domain.GitCommit
	for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "|", 4)
		if len(parts) != 4 {
			continue
		}
		t, _ := time.Parse(time.RFC3339, strings.TrimSpace(parts[3]))
		commits = append(commits, domain.GitCommit{
			SHA:     parts[0],
			Message: parts[1],
			Author:  parts[2],
			Date:    t,
		})
	}
	if commits == nil {
		commits = []domain.GitCommit{}
	}
	return commits, nil
}

func (u *gitUsecase) Diff(
	ctx context.Context,
	taskID string,
) (string, error) {
	task, err := u.taskRepo.Get(ctx, taskID)
	if err != nil {
		return "", err
	}

	var diffArgs []string
	if task.BaseBranch != "" {
		diffArgs = []string{"diff", task.BaseBranch + "..HEAD", "--unified=3"}
	} else {
		diffArgs = []string{"diff", "HEAD", "--unified=3"}
	}
	out, err := runGitCmd(ctx, task.WorktreePath, diffArgs...)
	if err != nil {
		return "", fmt.Errorf("git usecase: diff: %w", err)
	}
	return out, nil
}

func (u *gitUsecase) ListFiles(
	ctx context.Context,
	taskID string,
) ([]string, error) {
	task, err := u.taskRepo.Get(ctx, taskID)
	if err != nil {
		return nil, err
	}

	out, err := runGitCmd(ctx, task.WorktreePath, "ls-files")
	if err != nil {
		return nil, fmt.Errorf("git usecase: list files: %w", err)
	}

	var files []string
	for _, f := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if f != "" {
			files = append(files, f)
		}
	}
	if files == nil {
		files = []string{}
	}
	return files, nil
}

func runGitCmd(
	ctx context.Context,
	dir string,
	args ...string,
) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return "", err
	}
	return out.String(), nil
}
