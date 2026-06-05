// Package conflicts detects, parses, and resolves merge conflict markers
// in a git working tree.
package conflicts

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// gitRunner is the function used to run git commands; overridable in tests.
var gitRunner = exec.Git

// ConflictedFiles returns the paths of files that have merge conflicts (04 §6).
func ConflictedFiles(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	r, err := gitRunner(ctx, repoPath, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return nil, fmt.Errorf("conflicts: conflicted files: %w", err)
	}
	if err := exec.RequireSuccess("conflicts: conflicted files", r); err != nil {
		return nil, err
	}

	raw := strings.TrimSpace(r.Stdout)
	if raw == "" {
		return nil, nil
	}

	lines := strings.Split(raw, "\n")
	paths := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			paths = append(paths, line)
		}
	}
	return paths, nil
}

// HasConflicts returns true if the repo currently has any unmerged paths.
func HasConflicts(
	ctx context.Context,
	repoPath string,
) (bool, error) {
	r, err := gitRunner(ctx, repoPath, "diff", "--name-only", "--diff-filter=U")
	if err != nil {
		return false, fmt.Errorf("conflicts: has conflicts: %w", err)
	}
	if err := exec.RequireSuccess("conflicts: has conflicts", r); err != nil {
		return false, err
	}
	return strings.TrimSpace(r.Stdout) != "", nil
}

// ParseFile parses conflict markers in a file and fetches three-way content
// from the git object store (git show :1:/:2:/:3:) (04 §6).
func ParseFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]domain.ConflictHunk, error) {
	absPath := filepath.Join(repoPath, filePath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("conflicts: parse file: read: %w", err)
	}

	blocks, err := extractConflictBlocks(string(data))
	if err != nil {
		return nil, fmt.Errorf("conflicts: parse file: %w", err)
	}

	hunks := make([]domain.ConflictHunk, 0, len(blocks))
	for _, b := range blocks {
		base, ourText, theirText, fetchErr := fetchThreeWay(ctx, repoPath, filePath, b)
		if fetchErr != nil {
			return nil, fmt.Errorf("conflicts: parse file: fetch: %w", fetchErr)
		}

		id := hunkID(filePath, b.startLine)
		hunks = append(hunks, domain.ConflictHunk{
			ID:         id,
			StartLine:  b.startLine,
			EndLine:    b.endLine,
			Ours:       ourText,
			Theirs:     theirText,
			Base:       base,
			Resolution: domain.ConflictResolutionUnresolved,
		})
	}
	return hunks, nil
}

// ResolveHunk writes resolved content to the file, replacing the conflict
// markers for the given hunk. When no conflict markers remain in the file,
// it also stages the file via `git add <path>` (04 §6).
func ResolveHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
	resolution domain.ConflictResolution,
	resolvedContent string,
) error {
	absPath := filepath.Join(repoPath, filePath)
	data, err := os.ReadFile(absPath)
	if err != nil {
		return fmt.Errorf("conflicts: resolve hunk: read: %w", err)
	}

	blocks, err := extractConflictBlocks(string(data))
	if err != nil {
		return fmt.Errorf("conflicts: resolve hunk: parse: %w", err)
	}

	target := findBlock(blocks, hunkID, filePath)
	if target == nil {
		return fmt.Errorf("conflicts: resolve hunk: hunk %q not found", hunkID)
	}

	replacement := resolvedText(target, resolution, resolvedContent)
	updated := replaceBlock(string(data), target, replacement)

	if err := os.WriteFile(absPath, []byte(updated), 0o644); err != nil {
		return fmt.Errorf("conflicts: resolve hunk: write: %w", err)
	}

	if hasMarkers(updated) {
		return nil
	}

	r, err := gitRunner(ctx, repoPath, "add", filePath)
	if err != nil {
		return fmt.Errorf("conflicts: resolve hunk: stage: %w", err)
	}
	return exec.RequireSuccess("conflicts: resolve hunk: stage", r)
}

type conflictBlock struct {
	startLine int
	endLine   int
	oursRaw   string
	baseRaw   string
	theirsRaw string
}

func extractConflictBlocks(
	content string,
) ([]conflictBlock, error) {
	lines := strings.Split(content, "\n")
	var blocks []conflictBlock

	i := 0
	for i < len(lines) {
		if !strings.HasPrefix(lines[i], "<<<<<<<") {
			i++
			continue
		}

		startLine := i + 1
		var ourLines, baseLines, theirLines []string
		section := "ours"
		i++

		for i < len(lines) {
			line := lines[i]

			if strings.HasPrefix(line, ">>>>>>>") {
				endLine := i + 1
				blocks = append(blocks, conflictBlock{
					startLine: startLine,
					endLine:   endLine,
					oursRaw:   strings.Join(ourLines, "\n"),
					baseRaw:   strings.Join(baseLines, "\n"),
					theirsRaw: strings.Join(theirLines, "\n"),
				})
				i++
				break
			}

			if strings.HasPrefix(line, "|||||||") {
				section = "base"
				i++
				continue
			}

			if strings.HasPrefix(line, "=======") {
				section = "theirs"
				i++
				continue
			}

			switch section {
			case "ours":
				ourLines = append(ourLines, line)
			case "base":
				baseLines = append(baseLines, line)
			case "theirs":
				theirLines = append(theirLines, line)
			}
			i++
		}
	}

	return blocks, nil
}

func fetchThreeWay(
	ctx context.Context,
	repoPath string,
	filePath string,
	b conflictBlock,
) (base, ours, theirs string, err error) {
	baseResult, err := exec.Git(ctx, repoPath, "show", fmt.Sprintf(":1:%s", filePath))
	if err != nil {
		return "", "", "", err
	}

	oursResult, err := exec.Git(ctx, repoPath, "show", fmt.Sprintf(":2:%s", filePath))
	if err != nil {
		return "", "", "", err
	}

	theirsResult, err := exec.Git(ctx, repoPath, "show", fmt.Sprintf(":3:%s", filePath))
	if err != nil {
		return "", "", "", err
	}

	baseContent := ""
	if baseResult.ExitCode == 0 {
		baseContent = extractLines(baseResult.Stdout, b.startLine, b.endLine)
		if baseContent == "" {
			baseContent = b.baseRaw
		}
	}

	oursContent := b.oursRaw
	if oursResult.ExitCode == 0 {
		extracted := extractLines(oursResult.Stdout, b.startLine, b.endLine)
		if extracted != "" {
			oursContent = extracted
		}
	}

	theirsContent := b.theirsRaw
	if theirsResult.ExitCode == 0 {
		extracted := extractLines(theirsResult.Stdout, b.startLine, b.endLine)
		if extracted != "" {
			theirsContent = extracted
		}
	}

	return baseContent, oursContent, theirsContent, nil
}

func extractLines(
	content string,
	startLine int,
	endLine int,
) string {
	lines := strings.Split(content, "\n")
	start := startLine - 1
	end := endLine - 1

	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}
	if start >= end || start >= len(lines) {
		return ""
	}
	return strings.Join(lines[start:end], "\n")
}

func hunkID(
	filePath string,
	startLine int,
) string {
	h := sha256.New()
	fmt.Fprintf(h, "%s%d", filePath, startLine)
	return fmt.Sprintf("%x", h.Sum(nil))[:12]
}

func findBlock(
	blocks []conflictBlock,
	id string,
	filePath string,
) *conflictBlock {
	for i := range blocks {
		if hunkID(filePath, blocks[i].startLine) == id {
			return &blocks[i]
		}
	}
	return nil
}

func resolvedText(
	b *conflictBlock,
	resolution domain.ConflictResolution,
	custom string,
) string {
	switch resolution {
	case domain.ConflictResolutionOurs:
		return b.oursRaw
	case domain.ConflictResolutionTheirs:
		return b.theirsRaw
	case domain.ConflictResolutionBoth:
		return b.oursRaw + "\n" + b.theirsRaw
	case domain.ConflictResolutionCustom:
		return custom
	default:
		return b.oursRaw
	}
}

func replaceBlock(
	content string,
	b *conflictBlock,
	replacement string,
) string {
	lines := strings.Split(content, "\n")

	start := b.startLine - 1
	end := b.endLine

	if start < 0 {
		start = 0
	}
	if end > len(lines) {
		end = len(lines)
	}

	var result []string
	result = append(result, lines[:start]...)

	if replacement != "" {
		result = append(result, strings.Split(replacement, "\n")...)
	}

	if end < len(lines) {
		result = append(result, lines[end:]...)
	}

	return strings.Join(result, "\n")
}

func hasMarkers(
	content string,
) bool {
	for _, line := range strings.Split(content, "\n") {
		if strings.HasPrefix(line, "<<<<<<<") {
			return true
		}
	}
	return false
}
