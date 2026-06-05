// Package conflicts detects, parses, and resolves merge conflict markers
// in a git working tree.
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

// gitRunner is the function used to run git commands; overridable in tests.
var gitRunner = exec.Git

// fileMu is a per-file mutex registry keyed by absolute file path.
var fileMu sync.Map // map[string]*sync.Mutex

func getFileMutex(absPath string) *sync.Mutex {
	actual, _ := fileMu.LoadOrStore(absPath, &sync.Mutex{})
	return actual.(*sync.Mutex)
}

// ConflictedFiles returns the paths of files that have merge conflicts (04 §6).
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

// HasConflicts returns true if the repo currently has any unmerged paths.
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

// ParseFile parses conflict markers in a file and fetches the common ancestor
// (base) content from the git object store (:1:<path>) (04 §6).
func ParseFile(
	ctx context.Context,
	repoPath string,
	filePath string,
) ([]gitdomain.ConflictHunk, error) {
	absPath := filepath.Join(repoPath, filePath)
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

	// Fetch the base (ancestor) content once per file — not once per block.
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

// ResolveHunk writes resolved content to the file, replacing the conflict
// markers for the given hunk. When no conflict markers remain in the file,
// it also stages the file via `git add <path>` (04 §6).
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

	// Stat the file first to preserve its permissions.
	info, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("conflicts: stat %s: %w", filePath, err)
	}
	mode := info.Mode()

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

// blockParser holds parsing state for extractConflictBlocks.
type blockParser struct {
	lines []string
}

// parseBlock processes lines starting at startIdx (the <<<<<<< line) until
// the matching >>>>>>> is found. It returns the completed block, the index of
// the line after >>>>>>>, and any error.
func (bp *blockParser) parseBlock(startIdx int) (conflictBlock, int, error) {
	lines := bp.lines
	startLine := startIdx + 1 // 1-based
	var ourLines, baseLines, theirLines []string
	section := "ours"
	i := startIdx + 1

	for i < len(lines) {
		line := lines[i]

		if strings.HasPrefix(line, ">>>>>>>") {
			endLine := i + 1 // 1-based
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

	// Exhausted lines without finding >>>>>>>.
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
		i = nextIdx - 1 // -1 because the loop will i++
	}

	return blocks, nil
}

// fetchBase fetches the common ancestor content (:1:<path>) from the git
// object store. Returns empty string if the file has no base stage (e.g. an
// add/add conflict).
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

// conflictHunkID generates a stable ID based on the content of the conflict
// block rather than its line position, so that edits above the block do not
// invalidate the ID.
func conflictHunkID(filePath string, oursRaw string, theirsRaw string) string {
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
	switch resolution {
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
