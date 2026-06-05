package diff

import (
	"context"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

var imageExtensions = map[string]bool{
	".png":  true,
	".jpg":  true,
	".jpeg": true,
	".gif":  true,
	".webp": true,
	".svg":  true,
	".bmp":  true,
}

// WorkingTree runs `git diff -M [--cached]` and parses the result.
// staged=true adds --cached to diff against the index instead of the working tree.
func WorkingTree(
	ctx context.Context,
	repoPath string,
	staged bool,
) ([]domain.FileDiff, error) {
	args := []string{"diff", "-M"}
	if staged {
		args = append(args, "--cached")
	}
	r, err := exec.Git(ctx, repoPath, args...)
	if err != nil {
		return nil, fmt.Errorf("diff: working tree: %w", err)
	}
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
) (domain.MultiFileDiff, error) {
	meta, err := fetchCommitMeta(ctx, repoPath, sha)
	if err != nil {
		return domain.MultiFileDiff{}, err
	}
	diffText, err := fetchCommitDiff(ctx, repoPath, sha)
	if err != nil {
		return domain.MultiFileDiff{}, err
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
) (domain.MultiFileDiff, error) {
	r, err := exec.Git(ctx, repoPath, "show", "--no-patch", "--format=%H%n%s%n%b%n---author---%n%an%n---date---%n%aI", sha)
	if err != nil {
		return domain.MultiFileDiff{}, fmt.Errorf("diff: commit meta: %w", err)
	}
	if err := exec.RequireSuccess("diff: commit meta", r); err != nil {
		return domain.MultiFileDiff{}, err
	}
	return parseCommitMeta(r.Stdout), nil
}

func parseCommitMeta(
	output string,
) domain.MultiFileDiff {
	const authorSep = "---author---"
	const dateSep = "---date---"

	var d domain.MultiFileDiff
	authorIdx := strings.Index(output, authorSep)
	dateIdx := strings.Index(output, dateSep)

	if authorIdx < 0 || dateIdx < 0 {
		return d
	}

	header := strings.TrimSpace(output[:authorIdx])
	authorBlock := strings.TrimSpace(output[authorIdx+len(authorSep) : dateIdx])
	dateBlock := strings.TrimSpace(output[dateIdx+len(dateSep):])

	headerLines := strings.SplitN(header, "\n", 3)
	if len(headerLines) >= 1 {
		d.CommitHash = strings.TrimSpace(headerLines[0])
	}
	if len(headerLines) >= 2 {
		d.CommitMessage = strings.TrimSpace(headerLines[1])
	}
	if len(headerLines) >= 3 {
		d.CommitDescription = strings.TrimSpace(headerLines[2])
	}
	d.CommitAuthor = strings.TrimSpace(authorBlock)

	dateStr := strings.TrimSpace(dateBlock)
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
		r, err := exec.Git(ctx, repoPath, "show", "--format=", sha)
		if err != nil {
			return "", fmt.Errorf("diff: commit diff (root): %w", err)
		}
		if err := exec.RequireSuccess("diff: commit diff (root)", r); err != nil {
			return "", err
		}
		return r.Stdout, nil
	}
	r, err := exec.Git(ctx, repoPath, "diff", "-M", sha+"^", sha)
	if err != nil {
		return "", fmt.Errorf("diff: commit diff: %w", err)
	}
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
	r, err := exec.Git(ctx, repoPath, "rev-parse", sha+"^")
	if err != nil {
		return true
	}
	return r.ExitCode != 0
}

func totals(
	files []domain.FileDiff,
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
) []domain.FileDiff {
	if strings.TrimSpace(text) == "" {
		return nil
	}
	sections := splitFileSections(text)
	files := make([]domain.FileDiff, 0, len(sections))
	for _, section := range sections {
		f := parseFileSection(ctx, repoPath, section)
		files = append(files, f)
	}
	return files
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
) domain.FileDiff {
	lines := strings.Split(section, "\n")
	var f domain.FileDiff
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
			old := strings.TrimPrefix(line, "--- ")
			if old == "/dev/null" {
				f.IsNew = true
			}
			if !f.IsRenamed {
				f.OldPath = stripABPrefix(old)
			}

		case strings.HasPrefix(line, "+++ "):
			new_ := strings.TrimPrefix(line, "+++ ")
			if new_ == "/dev/null" {
				f.IsDeleted = true
			}
			if !f.IsRenamed {
				f.NewPath = stripABPrefix(new_)
				if f.FilePath == "" {
					f.FilePath = f.NewPath
				}
			}

		case strings.HasPrefix(line, "index ") && strings.Contains(line, ".."):
			oldBlobSHA, newBlobSHA = parseIndexLine(line)

		case strings.Contains(line, "Binary files") && strings.Contains(line, "differ"):
			f.IsBinary = true
			inHunk = false

		case strings.HasPrefix(line, "@@ "):
			if inHunk && len(hunkLines) > 0 {
				f.Lines = append(f.Lines, buildHunkLines(hunkLines, f.FilePath, len(f.Lines))...)
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
		f.Lines = append(f.Lines, buildHunkLines(hunkLines, f.FilePath, len(f.Lines))...)
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
	parts := strings.Fields(line)
	if len(parts) < 4 {
		return ""
	}
	bPath := parts[3]
	return strings.TrimPrefix(bPath, "b/")
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

func buildHunkLines(
	rawLines []string,
	filePath string,
	baseIndex int,
) []domain.DiffLine {
	if len(rawLines) == 0 {
		return nil
	}
	header := rawLines[0]
	bodyLines := rawLines[1:]

	oldLine, newLine := parseHunkHeader(header)

	bodyText := buildHunkBody(bodyLines)
	hunkID := HunkID(filePath, bodyText)

	var result []domain.DiffLine

	headerLine := domain.DiffLine{
		LineType: domain.DiffLineHeader,
		Content:  header,
	}
	result = append(result, headerLine)

	for _, raw := range bodyLines {
		if raw == "" || raw == "\\ No newline at end of file" {
			continue
		}
		dl := buildDiffLine(raw, hunkID, &oldLine, &newLine)
		result = append(result, dl)
	}

	return result
}

func buildHunkBody(
	bodyLines []string,
) string {
	var parts []string
	for _, line := range bodyLines {
		if line == "" || line == "\\ No newline at end of file" {
			continue
		}
		if len(line) == 0 {
			continue
		}
		ch := line[0]
		if ch == '+' || ch == '-' || ch == ' ' {
			parts = append(parts, line)
		}
	}
	return strings.Join(parts, "\n")
}

func buildDiffLine(
	raw string,
	hunkID string,
	oldLine *int,
	newLine *int,
) domain.DiffLine {
	if len(raw) == 0 {
		return domain.DiffLine{}
	}
	ch := raw[0]
	content := raw[1:]

	switch ch {
	case '+':
		n := *newLine
		*newLine++
		return domain.DiffLine{
			LineType:      domain.DiffLineAdded,
			Content:       content,
			NewLineNumber: &n,
			HunkID:        hunkID,
		}
	case '-':
		o := *oldLine
		*oldLine++
		return domain.DiffLine{
			LineType:      domain.DiffLineRemoved,
			Content:       content,
			OldLineNumber: &o,
			HunkID:        hunkID,
		}
	case ' ':
		o := *oldLine
		n := *newLine
		*oldLine++
		*newLine++
		return domain.DiffLine{
			LineType:      domain.DiffLineContext,
			Content:       content,
			OldLineNumber: &o,
			NewLineNumber: &n,
		}
	}
	return domain.DiffLine{
		LineType: domain.DiffLineContext,
		Content:  raw,
	}
}

func parseHunkHeader(
	header string,
) (int, int) {
	start := strings.Index(header, "-")
	if start < 0 {
		return 1, 1
	}
	rest := header[start+1:]
	end := strings.IndexAny(rest, " ,")
	if end < 0 {
		end = len(rest)
	}
	oldStart, _ := strconv.Atoi(rest[:end])

	plusIdx := strings.Index(header, "+")
	if plusIdx < 0 {
		return oldStart, 1
	}
	rest2 := header[plusIdx+1:]
	end2 := strings.IndexAny(rest2, " ,")
	if end2 < 0 {
		end2 = len(rest2)
	}
	newStart, _ := strconv.Atoi(rest2[:end2])

	return oldStart, newStart
}

func finalizeFileDiff(
	f domain.FileDiff,
) domain.FileDiff {
	f.IsImage = isImagePath(f.FilePath)
	f.Hunks = buildHunks(f.Lines)
	for _, line := range f.Lines {
		switch line.LineType {
		case domain.DiffLineAdded:
			f.Additions++
		case domain.DiffLineRemoved:
			f.Deletions++
		}
	}
	return f
}

func isImagePath(
	path string,
) bool {
	lower := strings.ToLower(path)
	for ext := range imageExtensions {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

func buildHunks(
	lines []domain.DiffLine,
) []domain.Hunk {
	var hunks []domain.Hunk
	var currentHunkID string
	currentStart := -1

	for i, line := range lines {
		if line.LineType == domain.DiffLineHeader {
			if currentHunkID != "" {
				hunks[len(hunks)-1].EndLine = i - 1
			}
			currentHunkID = ""
			currentStart = i
			continue
		}
		if line.HunkID != "" && line.HunkID != currentHunkID {
			currentHunkID = line.HunkID
			hunks = append(hunks, domain.Hunk{
				HunkID:    currentHunkID,
				Header:    headerForHunk(lines, currentStart),
				StartLine: currentStart,
				EndLine:   i,
			})
			continue
		}
		if len(hunks) > 0 {
			hunks[len(hunks)-1].EndLine = i
		}
	}
	return hunks
}

func headerForHunk(
	lines []domain.DiffLine,
	headerIdx int,
) string {
	if headerIdx < 0 || headerIdx >= len(lines) {
		return ""
	}
	if lines[headerIdx].LineType == domain.DiffLineHeader {
		return lines[headerIdx].Content
	}
	return ""
}

func populateBlobData(
	ctx context.Context,
	repoPath string,
	f domain.FileDiff,
	oldSHA string,
	newSHA string,
) domain.FileDiff {
	if !f.IsBinary && !f.IsImage {
		return f
	}
	if oldSHA != "" && !isZeroSHA(oldSHA) {
		data, err := fetchBlob(ctx, repoPath, oldSHA)
		if err == nil {
			f.OldBlobBase64 = data
		}
	}
	if newSHA != "" && !isZeroSHA(newSHA) {
		data, err := fetchBlob(ctx, repoPath, newSHA)
		if err == nil {
			f.NewBlobBase64 = data
		}
	}
	return f
}

func isZeroSHA(
	sha string,
) bool {
	for _, c := range sha {
		if c != '0' {
			return false
		}
	}
	return true
}

func fetchBlob(
	ctx context.Context,
	repoPath string,
	sha string,
) (string, error) {
	r, err := exec.Git(ctx, repoPath, "show", sha)
	if err != nil {
		return "", fmt.Errorf("diff: fetch blob %s: %w", sha, err)
	}
	if err := exec.RequireSuccess("diff: fetch blob", r); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString([]byte(r.Stdout)), nil
}
