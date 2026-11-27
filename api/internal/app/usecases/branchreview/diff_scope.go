package branchreview

import (
	"context"
	"fmt"
	"regexp"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var commitSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

func (u *branchReviewUsecase) resolveScopeRef(
	ctx context.Context,
	ws domain.Workspace,
	commit string,
) (string, error) {
	if commit != "" && !commitSHAPattern.MatchString(commit) {
		return "", fmt.Errorf(
			"branch review: %w: commit must be a hex object name",
			problem.ErrInvalidArgument,
		)
	}
	if ws.Provisioning == domain.WorkspacePlaceholder {
		return "", fmt.Errorf("branch review: %w (%s)", ErrWorkspaceUnprovisioned, ws.Branch)
	}
	if commit == "" {
		return u.resolveDiffRef(ctx, ws)
	}
	return commitRange(ctx, u.git, ws.WorktreePath, commit), nil
}

func commitRange(
	ctx context.Context,
	git revParser,
	worktreePath string,
	commit string,
) string {
	if parent, err := git.RevParse(ctx, worktreePath, commit+"^"); err == nil && parent != "" {
		return parent + ".." + commit
	}
	return emptyTreeSHA + ".." + commit
}

type revParser interface {
	RevParse(ctx context.Context, repoPath, rev string) (string, error)
}

func isCommitScoped(commit string) bool {
	return commit != ""
}
