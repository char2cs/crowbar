package diff

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
)

// ErrStaleHunk is returned when hunkID no longer matches any current diff hunk.
// The frontend re-fetches the diff and retries (04 §4).
var ErrStaleHunk = errors.New("diff: stale_hunk: hunk not found in current diff")

// HunkPatch returns a minimal patch containing only the hunk identified by
// hunkID, suitable for piping to `git apply --cached` (04 §4).
// staged=true re-diffs the staged area (for unstaging); false diffs the working tree (for staging).
// It re-diffs to account for any edits since the diff was fetched.
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
	files []domain.FileDiff,
	filePath string,
) *domain.FileDiff {
	for i := range files {
		if files[i].FilePath == filePath {
			return &files[i]
		}
	}
	return nil
}

func findHunkByID(
	hunks []domain.Hunk,
	hunkID string,
) *domain.Hunk {
	for i := range hunks {
		if hunks[i].HunkID == hunkID {
			return &hunks[i]
		}
	}
	return nil
}

func buildPatch(
	f *domain.FileDiff,
	hunk *domain.Hunk,
	lines []domain.DiffLine,
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
	if f.IsNew {
		sb.WriteString("new file mode 100644\n")
		fmt.Fprintf(&sb, "--- /dev/null\n")
		fmt.Fprintf(&sb, "+++ b/%s\n", newPath)
	} else if f.IsDeleted {
		sb.WriteString("deleted file mode 100644\n")
		fmt.Fprintf(&sb, "--- a/%s\n", oldPath)
		sb.WriteString("+++ /dev/null\n")
	} else {
		fmt.Fprintf(&sb, "--- a/%s\n", oldPath)
		fmt.Fprintf(&sb, "+++ b/%s\n", newPath)
	}

	hunkLines := lines[hunk.StartLine : hunk.EndLine+1]
	sb.WriteString(hunkHeader(hunkLines, hunk.Header))
	sb.WriteString("\n")

	for _, line := range hunkLines {
		if line.LineType == domain.DiffLineHeader {
			continue
		}
		switch line.LineType {
		case domain.DiffLineAdded:
			fmt.Fprintf(&sb, "+%s\n", line.Content)
		case domain.DiffLineRemoved:
			fmt.Fprintf(&sb, "-%s\n", line.Content)
		default:
			fmt.Fprintf(&sb, " %s\n", line.Content)
		}
	}

	return sb.String()
}

func hunkHeader(
	lines []domain.DiffLine,
	fallback string,
) string {
	for _, line := range lines {
		if line.LineType == domain.DiffLineHeader {
			return line.Content
		}
	}
	return fallback
}
