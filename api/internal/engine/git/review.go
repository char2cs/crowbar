package git

import (
	"context"
	"io"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

type reviewReads interface {
	RangeDiff(
		ctx context.Context,
		repoPath string,
		base string,
		branch string,
	) (gitdomain.MultiFileDiff, error)

	DiffAgainstRef(
		ctx context.Context,
		repoPath string,
		ref string,
	) (gitdomain.MultiFileDiff, error)

	ReviewFiles(
		ctx context.Context,
		repoPath string,
		ref string,
		dirty []string,
	) ([]gitdomain.ReviewFileSummary, error)

	ReviewFilePatch(
		ctx context.Context,
		repoPath string,
		ref string,
		path string,
		maxLines int,
		w io.Writer,
	) (int, bool, error)

	ReviewFileSides(ctx context.Context, repoPath, ref, path, oldPath string) (gitdomain.ReviewFileSides, error)

	ReviewOutline(
		ctx context.Context,
		repoPath string,
		ref string,
	) ([]gitdomain.FileOutline, error)

	ReviewSearch(
		ctx context.Context,
		repoPath string,
		ref string,
		query string,
		opts gitdomain.SearchOpts,
	) ([]gitdomain.SearchHit, bool, error)
}
