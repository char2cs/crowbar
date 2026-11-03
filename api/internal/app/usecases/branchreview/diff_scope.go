package branchreview

import (
	"context"
	"fmt"
	"regexp"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// commitSHAPattern is the only shape a scoping commit may take: hex-only because the ref becomes a
// `git diff` argument and a leading `-` would be read as a flag. Abbreviated SHAs are allowed.
var commitSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// emptyTreeSHA is git's SHA-1 empty tree, which a root commit is diffed against. sha256 repos are a
// deliberate limit: supporting them would cost a hash-object round trip on every commit diff.
const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// resolveScopeRef resolves the review ref: "" is the branch diff incl. working tree; a commit becomes
// `<parent>..<commit>`, two immutable trees (hence cacheable), since a lone ref diffs against the
// working tree. A placeholder workspace has no worktree to run git in and is refused here.
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

// commitRange builds the `<base>..<commit>` range for one commit, falling back
// to the empty tree when the commit has no parent (a root commit, where
// `<commit>^` does not resolve).
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

// revParser is the sliver of the git port commitRange needs, named so the
// helper can be tested without standing up the whole engine.
type revParser interface {
	RevParse(ctx context.Context, repoPath, rev string) (string, error)
}

// isCommitScoped reports whether a scope pins the read to immutable trees, and
// therefore whether the working tree can affect its result.
func isCommitScoped(commit string) bool {
	return commit != ""
}
