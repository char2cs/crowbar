package git

import (
	"context"
	"io"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// reviewReads is the branch-review surface of Engine: diffs against a base ref, read
// per file and streamed so a huge branch is never held whole.
type reviewReads interface {
	// RangeDiff returns the three-dot diff between base and branch (09 §2).
	// Uses `git diff -M <base>...<branch>` to show commits reachable from
	// branch but not from base. Commit metadata fields are always zero-value.
	RangeDiff(
		ctx context.Context,
		repoPath string,
		base string,
		branch string,
	) (gitdomain.MultiFileDiff, error)

	// DiffAgainstRef returns the working-tree-inclusive diff against ref:
	// committed changes since ref plus uncommitted tracked modifications
	// (`git diff -M <ref> --`). Used for the blended branch-review diff.
	DiffAgainstRef(
		ctx context.Context,
		repoPath string,
		ref string,
	) (gitdomain.MultiFileDiff, error)

	// ReviewFiles returns DiffAgainstRef's file set as per-file status and line counts, no hunk
	// content. dirty, the paths git status reports differ from HEAD, keeps counts off the O(diff)
	// path; nil means unknown and recomputes every count.
	ReviewFiles(
		ctx context.Context,
		repoPath string,
		ref string,
		dirty []string,
	) ([]gitdomain.ReviewFileSummary, error)

	// ReviewFilePatch streams one file's (by NEW name) unified patch into w, so a huge branch diff is
	// never materialised. maxLines <= 0 is unlimited; a cap cuts on a hunk boundary so the client can
	// still parse it. A path with no changes is an empty patch, not an error.
	ReviewFilePatch(
		ctx context.Context,
		repoPath string,
		ref string,
		path string,
		maxLines int,
		w io.Writer,
	) (int, bool, error)

	// ReviewFileSides is one file's full old and new text against ref, for the lines a patch omits;
	// oldPath is a rename source ("" means path), and an oversized side fails ErrFileTooLarge.
	ReviewFileSides(ctx context.Context, repoPath, ref, path, oldPath string) (gitdomain.ReviewFileSides, error)

	// ReviewOutline returns per-file hunk geometry against ref with no content, streamed and discarded
	// as read: O(hunks) rather than O(lines), so the client can reserve scroll space before any patch.
	ReviewOutline(
		ctx context.Context,
		repoPath string,
		ref string,
	) ([]gitdomain.FileOutline, error)

	// ReviewSearch finds query in the diff CONTENT against ref, returning file and line of each match
	// and whether opts.Limit cut it short. Streamed line by line and stopped when the limit fills, so
	// cost is bounded by hits asked for; the client only holds a window and cannot search locally.
	ReviewSearch(
		ctx context.Context,
		repoPath string,
		ref string,
		query string,
		opts gitdomain.SearchOpts,
	) ([]gitdomain.SearchHit, bool, error)
}
