package diff

import (
	"context"
	"fmt"
	"strings"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ErrStaleHunk is returned when hunkID matches no current diff hunk; the frontend
// re-fetches the diff and retries.
var ErrStaleHunk = problem.New(problem.Conflict, "diff: stale_hunk: hunk not found in current diff")

// HunkPatch returns a minimal patch holding only hunkID's hunk, for `git apply --cached`.
// It re-diffs (the staged area when staged, else the working tree) to account for edits
// since the diff was fetched.
func HunkPatch(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
	staged bool,
) (string, error) {
	files, err := WorkingTree(ctx, repoPath, staged)
	if err != nil {
		return "", fmt.Errorf("diff: hunk patch: re-diff: %w", err)
	}

	target := findFileInDiff(files, filePath)
	if target == nil {
		return "", ErrStaleHunk
	}

	hunk := findHunkByID(target.Hunks, hunkID)
	if hunk == nil {
		return "", ErrStaleHunk
	}

	patch := buildPatch(target, hunk, target.Lines)
	return patch, nil
}

func findFileInDiff(
	files []gitdomain.FileDiff,
	filePath string,
) *gitdomain.FileDiff {
	for i := range files {
		if files[i].FilePath == filePath {
			return &files[i]
		}
	}
	return nil
}

func findHunkByID(
	hunks []gitdomain.Hunk,
	hunkID string,
) *gitdomain.Hunk {
	for i := range hunks {
		if hunks[i].HunkID == hunkID {
			return &hunks[i]
		}
	}
	return nil
}

func buildPatch(
	f *gitdomain.FileDiff,
	hunk *gitdomain.Hunk,
	lines []gitdomain.DiffLine,
) string {
	var sb strings.Builder

	oldPath := f.FilePath
	newPath := f.FilePath
	if f.OldPath != "" {
		oldPath = f.OldPath
	}
	if f.NewPath != "" {
		newPath = f.NewPath
	}

	fmt.Fprintf(&sb, "diff --git a/%s b/%s\n", oldPath, newPath)
	switch {
	case f.IsNew:
		sb.WriteString("new file mode 100644\n")
		fmt.Fprintf(&sb, "--- /dev/null\n")
		fmt.Fprintf(&sb, "+++ b/%s\n", newPath)
	case f.IsDeleted:
		sb.WriteString("deleted file mode 100644\n")
		fmt.Fprintf(&sb, "--- a/%s\n", oldPath)
		sb.WriteString("+++ /dev/null\n")
	default:
		fmt.Fprintf(&sb, "--- a/%s\n", oldPath)
		fmt.Fprintf(&sb, "+++ b/%s\n", newPath)
	}

	hunkLines := lines[hunk.StartLine : hunk.EndLine+1]
	sb.WriteString(hunkHeader(hunkLines, hunk.Header))
	sb.WriteString("\n")

	for _, line := range hunkLines {
		if line.LineType == gitdomain.DiffLineHeader {
			continue
		}
		switch line.LineType { //nolint:exhaustive // default renders context lines; header lines are skipped above.
		case gitdomain.DiffLineAdded:
			fmt.Fprintf(&sb, "+%s\n", line.Content)
		case gitdomain.DiffLineRemoved:
			fmt.Fprintf(&sb, "-%s\n", line.Content)
		default:
			fmt.Fprintf(&sb, " %s\n", line.Content)
		}
	}

	return sb.String()
}

func hunkHeader(
	lines []gitdomain.DiffLine,
	fallback string,
) string {
	for _, line := range lines {
		if line.LineType == gitdomain.DiffLineHeader {
			return line.Content
		}
	}
	return fallback
}
