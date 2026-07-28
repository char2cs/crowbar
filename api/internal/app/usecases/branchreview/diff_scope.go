package branchreview

import (
	"context"
	"fmt"
	"regexp"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// commitSHAPattern is the only shape a scoping commit may take.
//
// Deliberately hex-only rather than "any valid ref": the ref this produces
// becomes an ARGUMENT to `git diff`, and git reads a leading `-` as a flag. A
// permissive pattern would put the caller one string away from passing
// `--output=/etc/passwd`. Hex also matches how the surface actually addresses a
// commit — it navigates from the history list, which carries SHAs — so nothing
// is given up. Abbreviated SHAs are allowed; git resolves them.
var commitSHAPattern = regexp.MustCompile(`^[0-9a-fA-F]{4,64}$`)

// emptyTreeSHA is git's canonical empty tree, the thing a root commit is
// diffed against — the same base `git show` uses when a commit has no parent.
//
// SHA-1 object format, which is git's default. A repository created with
// `--object-format=sha256` has a different empty tree and a root commit there
// will fail to diff; that is a deliberate limit rather than an oversight,
// because the alternative is a `hash-object` round trip on every commit diff to
// serve a case that needs it only for the very first commit of a sha256 repo.
const emptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

// resolveScopeRef resolves the ref the windowed review reads.
//
// An empty commit means the workspace's branch diff — everything this branch
// carries since it forked, working tree included. That is the default and the
// GitHub-like review step the surface exists for.
//
// A non-empty commit narrows to that ONE commit, expressed as the range
// `<parent>..<commit>`. The range form matters: every read underneath
// (`Outline`, `FilePatch`, `SearchDiff`) passes its ref to `git diff <ref> --`,
// which against a single ref means "ref versus the WORKING TREE". For a commit
// that would answer "everything since that commit", the opposite of what the
// reader asked for. Naming both ends pins the diff to two immutable trees.
//
// Being a diff of two immutable trees is also why a commit-scoped read is
// cacheable regardless of the working tree — see cacheableOutlineKey.
func (u *branchReviewUsecase) resolveScopeRef(
	ctx context.Context,
	ws domain.Workspace,
	commit string,
) (string, error) {
	if commit == "" {
		return u.resolveDiffRef(ctx, ws)
	}
	if !commitSHAPattern.MatchString(commit) {
		return "", fmt.Errorf(
			"branch review: %w: commit must be a hex object name",
			apperr.ErrInvalidArgument,
		)
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
	RevParse(ctx context.Context, repoPath string, rev string) (string, error)
}

// isCommitScoped reports whether a scope pins the read to immutable trees, and
// therefore whether the working tree can affect its result.
func isCommitScoped(commit string) bool {
	return commit != ""
}
