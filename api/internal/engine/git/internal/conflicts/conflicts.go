package conflicts

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

var gitRunner = exec.Git

var fileMu sync.Map

func getFileMutex(absPath string) *sync.Mutex {
	actual, _ := fileMu.LoadOrStore(absPath, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

func ConflictedFiles(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	r := gitRunner(ctx, repoPath, "diff", "--name-only", "--diff-filter=U")
	if err := exec.RequireSuccess("conflicts: conflicted files", r); err != nil {
		return nil, fmt.Errorf("conflicts: conflicted files: %w", err)
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

func HasConflicts(
	ctx context.Context,
	repoPath string,
) (bool, error) {
	r := gitRunner(ctx, repoPath, "diff", "--name-only", "--diff-filter=U")
	if err := exec.RequireSuccess("conflicts: has conflicts", r); err != nil {
		return false, fmt.Errorf("conflicts: has conflicts: %w", err)
	}
	return strings.TrimSpace(r.Stdout) != "", nil
}

func ParseFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]gitdomain.ConflictHunk, error) {
	absPath := filepath.Join(repoPath, filePath)
	//nolint:gosec // G304: reads a working-tree file under the repo path by design.
	data, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("conflicts: parse file: read: %w", err)
	}

	blocks, err := extractConflictBlocks(string(data))
	if err != nil {
		return nil, fmt.Errorf("conflicts: parse file: %w", err)
	}

	if len(blocks) == 0 {
		return nil, nil
	}

	baseContent, fetchErr := fetchBase(ctx, repoPath, filePath)
	if fetchErr != nil {
		return nil, fmt.Errorf("conflicts: parse file: fetch base: %w", fetchErr)
	}

	hunks := make([]gitdomain.ConflictHunk, 0, len(blocks))
	for _, b := range blocks {
		id := conflictHunkID(filePath, b.oursRaw, b.theirsRaw)
		hunks = append(hunks, gitdomain.ConflictHunk{
			ID:         id,
			StartLine:  b.startLine,
			EndLine:    b.endLine,
			Ours:       b.oursRaw,
			Theirs:     b.theirsRaw,
			Base:       baseContent,
			Resolution: gitdomain.ConflictResolutionUnresolved,
		})
	}
	return hunks, nil
}

func ResolveHunk(
	ctx context.Context,
	repoPath string,
	filePath string,
	hunkID string,
	resolution gitdomain.ConflictResolution,
	resolvedContent string,
) error {
	absPath := filepath.Join(repoPath, filePath)

	mu := getFileMutex(absPath)
	mu.Lock()
	defer mu.Unlock()

	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("conflicts: stat %s: %w", filePath, err)
	}
	mode := info.Mode()

	//nolint:gosec // G304: reads a working-tree file under the repo path by design.
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

	replacement, err := resolvedText(target, resolution, resolvedContent)
	if err != nil {
		return fmt.Errorf("conflicts: resolve hunk: %w", err)
	}
	updated := replaceBlock(string(data), target, replacement)

	//nolint:gosec // G703: writes back to the working-tree file under the repo path by design.
	if err := os.WriteFile(absPath, []byte(updated), mode); err != nil {
		return fmt.Errorf("conflicts: resolve hunk: write: %w", err)
	}

	if hasMarkers(updated) {
		return nil
	}

	r := gitRunner(ctx, repoPath, "add", filePath)
	return exec.RequireSuccess("conflicts: resolve hunk: stage", r)
}

type conflictBlock struct {
	startLine int
	endLine   int
	oursRaw   string
	baseRaw   string
	theirsRaw string
}

type blockParser struct {
	lines []string
}

func (bp *blockParser) parseBlock(startIdx int) (conflictBlock, int, error) {
	lines := bp.lines
	startLine := startIdx + 1
	var ourLines, baseLines, theirLines []string
	section := "ours"
	i := startIdx + 1

	for i < len(lines) {
		line := lines[i]

		if strings.HasPrefix(line, ">>>>>>>") {
			endLine := i + 1
			b := conflictBlock{
				startLine: startLine,
				endLine:   endLine,
				oursRaw:   strings.Join(ourLines, "\n"),
				baseRaw:   strings.Join(baseLines, "\n"),
				theirsRaw: strings.Join(theirLines, "\n"),
			}
			return b, i + 1, nil
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

	return conflictBlock{}, i, fmt.Errorf("conflicts: unclosed conflict marker starting at line %d in file", startLine)
}

func extractConflictBlocks(
	content string,
) ([]conflictBlock, error) {
	lines := strings.Split(content, "\n")
	bp := &blockParser{lines: lines}
	var blocks []conflictBlock

	for i := 0; i < len(lines); i++ {
		if !strings.HasPrefix(lines[i], "<<<<<<<") {
			continue
		}
		block, nextIdx, err := bp.parseBlock(i)
		if err != nil {
			return nil, fmt.Errorf("conflicts: extract blocks: %w", err)
		}
		blocks = append(blocks, block)
		i = nextIdx - 1
	}

	return blocks, nil
}

func fetchBase(
	ctx context.Context,
	repoPath string,
	filePath string,
) (string, error) {
	baseResult := exec.Git(ctx, repoPath, "show", fmt.Sprintf(":1:%s", filePath))
	if baseResult.ExitCode != 0 {
		return "", nil
	}
	return baseResult.Stdout, nil
}

func conflictHunkID(filePath, oursRaw, theirsRaw string) string {
	h := sha256.New()
	h.Write([]byte(filePath))
	h.Write([]byte(oursRaw))
	h.Write([]byte(theirsRaw))
	return hex.EncodeToString(h.Sum(nil))[:12]
}

func findBlock(
	blocks []conflictBlock,
	id string,
	filePath string,
) *conflictBlock {
	for i := range blocks {
		if conflictHunkID(filePath, blocks[i].oursRaw, blocks[i].theirsRaw) == id {
			return &blocks[i]
		}
	}
	return nil
}

func resolvedText(
	b *conflictBlock,
	resolution gitdomain.ConflictResolution,
	custom string,
) (string, error) {
	switch resolution { //nolint:exhaustive // default rejects Unresolved and any unknown resolution.
	case gitdomain.ConflictResolutionOurs:
		return b.oursRaw, nil
	case gitdomain.ConflictResolutionTheirs:
		return b.theirsRaw, nil
	case gitdomain.ConflictResolutionBoth:
		return b.oursRaw + "\n" + b.theirsRaw, nil
	case gitdomain.ConflictResolutionCustom:
		return custom, nil
	default:
		return "", fmt.Errorf("conflicts: unknown resolution %q", resolution)
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
