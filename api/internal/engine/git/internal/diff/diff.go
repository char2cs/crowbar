package diff

import (
	"context"
	"strings"
	"time"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// WorkingTree runs `git diff -M [--cached]` and parses the result.
// staged=true adds --cached to diff against the index instead of the working tree.
func WorkingTree(
	ctx context.Context,
	repoPath string,
	staged bool,
) ([]gitdomain.FileDiff, error) {
	args := []string{"diff", "-M"}
	if staged {
		args = append(args, "--cached")
	}
	r := exec.Git(ctx, repoPath, args...)
	if err := exec.RequireSuccess("diff: working tree", r); err != nil {
		return nil, err
	}
	return parseFiles(ctx, repoPath, r.Stdout), nil
}

// Commit parses the diff for a single commit.
// For the root commit (no parent) it uses `git show --format= <sha>`.
// For all other commits it uses `git diff -M <sha>^ <sha>`.
func Commit(
	ctx context.Context,
	repoPath string,
	sha string,
) (gitdomain.MultiFileDiff, error) {
	meta, err := fetchCommitMeta(ctx, repoPath, sha)
	if err != nil {
		return gitdomain.MultiFileDiff{}, err
	}
	diffText, err := fetchCommitDiff(ctx, repoPath, sha)
	if err != nil {
		return gitdomain.MultiFileDiff{}, err
	}
	files := parseFiles(ctx, repoPath, diffText)
	totalAdd, totalDel := totals(files)
	meta.Files = files
	meta.TotalFiles = len(files)
	meta.TotalAdditions = totalAdd
	meta.TotalDeletions = totalDel
	return meta, nil
}

func fetchCommitMeta(
	ctx context.Context,
	repoPath string,
	sha string,
) (gitdomain.MultiFileDiff, error) {
	r := exec.Git(ctx, repoPath, "show", "--no-patch", "--format=%H%x00%s%x00%b%x00%an%x00%aI", sha)
	if err := exec.RequireSuccess("diff: commit meta", r); err != nil {
		return gitdomain.MultiFileDiff{}, err
	}
	return parseCommitMeta(r.Stdout), nil
}

func parseCommitMeta(
	output string,
) gitdomain.MultiFileDiff {
	var d gitdomain.MultiFileDiff
	// Format is: hash NUL subject NUL body NUL author NUL date
	parts := strings.SplitN(strings.TrimRight(output, "\n"), "\x00", 5)
	if len(parts) < 5 {
		return d
	}
	d.CommitHash = strings.TrimSpace(parts[0])
	d.CommitMessage = strings.TrimSpace(parts[1])
	d.CommitDescription = strings.TrimSpace(parts[2])
	d.CommitAuthor = strings.TrimSpace(parts[3])
	dateStr := strings.TrimSpace(parts[4])
	if dateStr != "" {
		t, err := time.Parse(time.RFC3339, dateStr)
		if err == nil {
			d.CommitDate = &t
		}
	}
	return d
}

func fetchCommitDiff(
	ctx context.Context,
	repoPath string,
	sha string,
) (string, error) {
	isRoot := isRootCommit(ctx, repoPath, sha)
	if isRoot {
		r := exec.Git(ctx, repoPath, "show", "--format=", sha)
		if err := exec.RequireSuccess("diff: commit diff (root)", r); err != nil {
			return "", err
		}
		return r.Stdout, nil
	}
	r := exec.Git(ctx, repoPath, "diff", "-M", sha+"^", sha)
	if err := exec.RequireSuccess("diff: commit diff", r); err != nil {
		return "", err
	}
	return r.Stdout, nil
}

func isRootCommit(
	ctx context.Context,
	repoPath string,
	sha string,
) bool {
	r := exec.Git(ctx, repoPath, "rev-parse", sha+"^")
	return r.ExitCode != 0
}

func totals(
	files []gitdomain.FileDiff,
) (int, int) {
	var add, del int
	for _, f := range files {
		add += f.Additions
		del += f.Deletions
	}
	return add, del
}

func parseFiles(
	ctx context.Context,
	repoPath string,
	text string,
) []gitdomain.FileDiff {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	sections := splitFileSections(text)
	var result []gitdomain.FileDiff
	for _, section := range sections {
		f := parseFileSection(ctx, repoPath, section)
		if f.FilePath != "" {
			result = append(result, f)
		}
	}
	return result
}

func splitFileSections(
	text string,
) []string {
	var sections []string
	var current strings.Builder
	lines := strings.Split(text, "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "diff --git ") && current.Len() > 0 {
			sections = append(sections, current.String())
			current.Reset()
		}
		current.WriteString(line)
		current.WriteByte('\n')
	}
	if current.Len() > 0 {
		sections = append(sections, current.String())
	}
	return sections
}

func parseFileSection(
	ctx context.Context,
	repoPath string,
	section string,
) gitdomain.FileDiff {
	lines := strings.Split(section, "\n")
	var f gitdomain.FileDiff
	var oldBlobSHA, newBlobSHA string
	var hunkLines []string
	inHunk := false

	for _, line := range lines {
		switch {
		case strings.HasPrefix(line, "diff --git "):
			f.FilePath = parseDiffGitPath(line)
			inHunk = false

		case strings.HasPrefix(line, "similarity index "):
			f.IsRenamed = true

		case strings.HasPrefix(line, "rename from "):
			f.OldPath = strings.TrimPrefix(line, "rename from ")

		case strings.HasPrefix(line, "rename to "):
			f.NewPath = strings.TrimPrefix(line, "rename to ")
			f.FilePath = f.NewPath

		case strings.HasPrefix(line, "--- "):
			applyOldPath(&f, strings.TrimPrefix(line, "--- "))

		case strings.HasPrefix(line, "+++ "):
			applyNewPath(&f, strings.TrimPrefix(line, "+++ "))

		case strings.HasPrefix(line, "index ") && strings.Contains(line, ".."):
			oldBlobSHA, newBlobSHA = parseIndexLine(line)

		case strings.Contains(line, "Binary files") && strings.Contains(line, "differ"):
			f.IsBinary = true
			inHunk = false

		case strings.HasPrefix(line, "@@ "):
			if inHunk && len(hunkLines) > 0 {
				f.Lines = append(f.Lines, buildHunkLines(hunkLines, f.FilePath)...)
			}
			hunkLines = []string{line}
			inHunk = true

		default:
			if inHunk {
				hunkLines = append(hunkLines, line)
			}
		}
	}

	if inHunk && len(hunkLines) > 0 {
		f.Lines = append(f.Lines, buildHunkLines(hunkLines, f.FilePath)...)
	}

	f = finalizeFileDiff(f)

	if f.FilePath == "" {
		f.FilePath = f.NewPath
	}
	if f.FilePath == "" {
		f.FilePath = f.OldPath
	}

	f = populateBlobData(ctx, repoPath, f, oldBlobSHA, newBlobSHA)
	return f
}

func parseDiffGitPath(
	line string,
) string {
	// line format: "diff --git a/<path> b/<path>"
	// Use the last " b/" as boundary since path may contain spaces.
	idx := strings.LastIndex(line, " b/")
	if idx < 0 {
		return ""
	}
	bPath := line[idx+3:] // strip leading " b/"
	return bPath
}

func applyOldPath(f *gitdomain.FileDiff, raw string) {
	old := stripABPrefix(raw)
	f.IsNew = old == "/dev/null"
	if !f.IsNew && !f.IsRenamed {
		f.OldPath = old
	}
}

func applyNewPath(f *gitdomain.FileDiff, raw string) {
	new_ := stripABPrefix(raw)
	f.IsDeleted = new_ == "/dev/null"
	if !f.IsDeleted && !f.IsRenamed {
		f.NewPath = new_
		if f.FilePath == "" {
			f.FilePath = new_
		}
	}
}

func stripABPrefix(
	path string,
) string {
	if strings.HasPrefix(path, "a/") {
		return strings.TrimPrefix(path, "a/")
	}
	if strings.HasPrefix(path, "b/") {
		return strings.TrimPrefix(path, "b/")
	}
	return path
}

func parseIndexLine(
	line string,
) (string, string) {
	line = strings.TrimPrefix(line, "index ")
	parts := strings.Fields(line)
	if len(parts) == 0 {
		return "", ""
	}
	shas := strings.SplitN(parts[0], "..", 2)
	if len(shas) != 2 {
		return "", ""
	}
	return shas[0], shas[1]
}

func finalizeFileDiff(
	f gitdomain.FileDiff,
) gitdomain.FileDiff {
	f.IsImage = isImagePath(f.FilePath)
	f.Hunks = buildHunks(f.Lines)
	for _, line := range f.Lines {
		switch line.LineType {
		case gitdomain.DiffLineAdded:
			f.Additions++
		case gitdomain.DiffLineRemoved:
			f.Deletions++
		}
	}
	return f
}
