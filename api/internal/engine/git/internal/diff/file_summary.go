package diff

import (
	"context"
	"maps"
	"strconv"
	"strings"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// pathspecChunkBytes bounds one dirty-count invocation's pathspec bytes well under the
// smallest ARG_MAX this daemon runs on (1 MiB on macOS).
var pathspecChunkBytes = 96 * 1024

// runGit is the seam the committed-count query reaches git through. Tests
// replace it to count invocations, so a cache hit is proven by an invocation
// count rather than by a timing measurement.
var runGit = exec.Git

// committedCounts is process-wide: what it holds depends only on a repo path
// and two commit SHAs, so every caller on a clone can share it.
var committedCounts = &summaryCache{}

// FileSummaries returns AgainstRef's file set (no hunks) with status from `--name-status -M` and
// counts from summaryCounts. dirty is git status's changed paths; nil recomputes every count.
// Untracked files are the caller's; binary counts are -1; renames carry OldPath.
func FileSummaries(
	ctx context.Context,
	repoPath string,
	ref string,
	dirty []string,
) ([]gitdomain.ReviewFileSummary, error) {
	nameStatus := exec.Git(ctx, repoPath, "diff", "--name-status", "-M", "-z", ref, "--")
	if err := exec.RequireSuccess("diff: file summary name-status", nameStatus); err != nil {
		return nil, err
	}
	entries := parseNameStatusZ(nameStatus.Stdout)
	counts, err := summaryCounts(ctx, repoPath, ref, entries, dirty)
	if err != nil {
		return nil, err
	}
	for i := range entries {
		count, ok := counts[entries[i].Path]
		if !ok {
			continue
		}
		entries[i].Additions = count.additions
		entries[i].Deletions = count.deletions
	}
	return entries, nil
}

type numCount struct {
	additions int
	deletions int
}

// summaryCounts avoids an O(diff) numstat per tick: ref->HEAD is cached on (repo, ref, headSHA) and
// only stale entries are re-diffed against the worktree, winning per path. dirty == nil disables the cache.
func summaryCounts(
	ctx context.Context,
	repoPath string,
	ref string,
	entries []gitdomain.ReviewFileSummary,
	dirty []string,
) (map[string]numCount, error) {
	if dirty == nil {
		return worktreeCounts(ctx, repoPath, ref)
	}
	// A range ref is a diff of two commits with no worktree half, and the split would hand git
	// three commits, which it refuses with exit 129.
	if isRangeRef(ref) {
		return worktreeCounts(ctx, repoPath, ref)
	}
	head, ok := headCommit(ctx, repoPath)
	if !ok {
		return worktreeCounts(ctx, repoPath, ref)
	}
	committed, err := committedCounts.get(
		summaryKey{repoPath: repoPath, ref: ref, headSHA: head},
		func() (map[string]numCount, error) { return treeCounts(ctx, repoPath, ref, head) },
	)
	if err != nil {
		return nil, err
	}
	stale := staleEntries(entries, dirty, committed)
	fresh, err := pathspecCounts(ctx, repoPath, ref, stale)
	if err != nil {
		return nil, err
	}
	return mergeCounts(entries, committed, stale, fresh), nil
}

// isRangeRef reports whether ref names both ends of a diff (`a..b`) rather than
// a single commit-ish to be diffed against the working tree.
func isRangeRef(
	ref string,
) bool {
	return strings.Contains(ref, "..")
}

// worktreeCounts is the unsplit, always-correct O(diff) query: the fallback when the split
// cannot be trusted, and the tests' equivalence oracle.
func worktreeCounts(
	ctx context.Context,
	repoPath string,
	ref string,
) (map[string]numCount, error) {
	r := exec.Git(ctx, repoPath, "diff", "--numstat", "-M", "-z", ref, "--")
	if err := exec.RequireSuccess("diff: file summary numstat", r); err != nil {
		return nil, err
	}
	return parseNumstatZ(r.Stdout), nil
}

func treeCounts(
	ctx context.Context,
	repoPath string,
	ref string,
	head string,
) (map[string]numCount, error) {
	r := runGit(ctx, repoPath, "diff", "--numstat", "-M", "-z", ref, head, "--")
	if err := exec.RequireSuccess("diff: file summary committed numstat", r); err != nil {
		return nil, err
	}
	return parseNumstatZ(r.Stdout), nil
}

func headCommit(
	ctx context.Context,
	repoPath string,
) (string, bool) {
	r := exec.Git(ctx, repoPath, "rev-parse", "HEAD")
	if r.ExitCode != 0 {
		return "", false
	}
	sha := strings.TrimSpace(r.Stdout)
	return sha, sha != ""
}

// staleEntries picks the entries whose committed count cannot be trusted, in
// name-status order. Rename candidates come first so chunkPathspecs can keep
// them in one invocation.
func staleEntries(
	entries []gitdomain.ReviewFileSummary,
	dirty []string,
	committed map[string]numCount,
) []gitdomain.ReviewFileSummary {
	dirtySet := make(map[string]bool, len(dirty))
	for _, p := range dirty {
		dirtySet[p] = true
	}
	var candidates, plain []gitdomain.ReviewFileSummary
	for _, e := range entries {
		if !isStale(e, dirtySet, committed) {
			continue
		}
		if isRenameCandidate(e) {
			candidates = append(candidates, e)
			continue
		}
		plain = append(plain, e)
	}
	return append(candidates, plain...)
}

// isStale: the entry is dirty, got no ref->HEAD count (renames pair differently), or has a
// name git status mangles, so its absence from the dirty set proves nothing.
func isStale(
	e gitdomain.ReviewFileSummary,
	dirty map[string]bool,
	committed map[string]numCount,
) bool {
	if dirty[e.Path] || (e.OldPath != "" && dirty[e.OldPath]) {
		return true
	}
	if _, ok := committed[e.Path]; !ok {
		return true
	}
	return !statusNamesPathExactly(e.Path)
}

// statusNamesPathExactly: porcelain v2 leaves spaces unescaped and C-quotes non-ASCII, so
// such a name's absence from the dirty set cannot be read as clean.
func statusNamesPathExactly(
	path string,
) bool {
	for i := range len(path) {
		c := path[i]
		if c <= ' ' || c > '~' || c == '"' || c == '\\' {
			return false
		}
	}
	return true
}

// isRenameCandidate reports whether git could pair this entry with another one
// as a rename. `-M` only ever pairs a deletion with an addition, so a plain
// modification is inert and can be moved to another invocation freely.
func isRenameCandidate(
	e gitdomain.ReviewFileSummary,
) bool {
	return e.Status == gitdomain.GitFileStatusAdded ||
		e.Status == gitdomain.GitFileStatusDeleted ||
		e.Status == gitdomain.GitFileStatusRenamed
}

// pathspecCounts re-diffs ref against the working tree for the stale entries
// alone. Nothing runs when nothing is stale, which is the whole point on a
// clean tree.
func pathspecCounts(
	ctx context.Context,
	repoPath string,
	ref string,
	stale []gitdomain.ReviewFileSummary,
) (map[string]numCount, error) {
	out := make(map[string]numCount, len(stale))
	for _, chunk := range chunkPathspecs(stale) {
		args := append([]string{"diff", "--numstat", "-M", "-z", ref, "--"}, chunk...)
		r := exec.Git(ctx, repoPath, args...)
		if err := exec.RequireSuccess("diff: file summary dirty numstat", r); err != nil {
			return nil, err
		}
		maps.Copy(out, parseNumstatZ(r.Stdout))
	}
	return out, nil
}

// chunkPathspecs keeps pathspecs under ARG_MAX; a rename's source travels with its destination
// and candidates come first, so only a single over-sized candidate block can be re-paired.
func chunkPathspecs(
	stale []gitdomain.ReviewFileSummary,
) [][]string {
	var chunks [][]string
	var current []string
	used := 0
	for _, e := range stale {
		specs := entryPathspecs(e)
		size := pathspecsSize(specs)
		if used+size > pathspecChunkBytes && len(current) > 0 {
			chunks = append(chunks, current)
			current, used = nil, 0
		}
		current = append(current, specs...)
		used += size
	}
	if len(current) > 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

// literalPathspec disables pathspec magic and glob interpretation, so a file
// literally named ":(icase)x" or "we*ird[1].txt" matches itself and nothing
// else, and anchors the match at the working-tree root the paths came from.
const literalPathspec = ":(top,literal)"

func entryPathspecs(
	e gitdomain.ReviewFileSummary,
) []string {
	if e.OldPath == "" {
		return []string{literalPathspec + e.Path}
	}
	return []string{literalPathspec + e.Path, literalPathspec + e.OldPath}
}

func pathspecsSize(
	specs []string,
) int {
	n := 0
	for _, s := range specs {
		n += len(s) + 1
	}
	return n
}

// mergeCounts joins the two halves with the recomputed value winning per path.
// A stale path missing from fresh must end with no count at all — never the
// committed one, which the working tree has moved out from under.
func mergeCounts(
	entries []gitdomain.ReviewFileSummary,
	committed map[string]numCount,
	stale []gitdomain.ReviewFileSummary,
	fresh map[string]numCount,
) map[string]numCount {
	out := make(map[string]numCount, len(entries))
	for _, e := range entries {
		count, ok := committed[e.Path]
		if !ok {
			continue
		}
		out[e.Path] = count
	}
	for _, e := range stale {
		delete(out, e.Path)
		count, ok := fresh[e.Path]
		if !ok {
			continue
		}
		out[e.Path] = count
	}
	return out
}

func parseNameStatusZ(
	text string,
) []gitdomain.ReviewFileSummary {
	tokens := strings.Split(text, "\x00")
	var out []gitdomain.ReviewFileSummary
	i := 0
	for i < len(tokens) {
		if tokens[i] == "" {
			i++
			continue
		}
		entry, next := nameStatusEntry(tokens, i)
		out = append(out, entry)
		i = next
	}
	return out
}

func nameStatusEntry(
	tokens []string,
	i int,
) (gitdomain.ReviewFileSummary, int) {
	code := tokens[i]
	if isRenameOrCopy(code) && i+2 < len(tokens) {
		return gitdomain.ReviewFileSummary{
			OldPath: tokens[i+1],
			Path:    tokens[i+2],
			Status:  gitdomain.GitFileStatusRenamed,
		}, i + 3
	}
	path := ""
	if i+1 < len(tokens) {
		path = tokens[i+1]
	}
	return gitdomain.ReviewFileSummary{
		Path:   path,
		Status: statusFromCode(code),
	}, i + 2
}

func parseNumstatZ(
	text string,
) map[string]numCount {
	tokens := strings.Split(text, "\x00")
	counts := make(map[string]numCount, len(tokens))
	i := 0
	for i < len(tokens) {
		if tokens[i] == "" {
			i++
			continue
		}
		path, count, next := numstatEntry(tokens, i)
		if path != "" {
			counts[path] = count
		}
		i = next
	}
	return counts
}

func numstatEntry(
	tokens []string,
	i int,
) (string, numCount, int) {
	fields := strings.SplitN(tokens[i], "\t", 3)
	if len(fields) < 3 {
		return "", numCount{}, i + 1
	}
	count := numCount{additions: parseCount(fields[0]), deletions: parseCount(fields[1])}
	if fields[2] != "" {
		return fields[2], count, i + 1
	}
	if i+2 < len(tokens) {
		return tokens[i+2], count, i + 3
	}
	return "", count, i + 1
}

func isRenameOrCopy(
	code string,
) bool {
	return strings.HasPrefix(code, "R") || strings.HasPrefix(code, "C")
}

func statusFromCode(
	code string,
) gitdomain.GitFileStatus {
	if code == "" {
		return gitdomain.GitFileStatusModified
	}
	switch code[0] {
	case 'A':
		return gitdomain.GitFileStatusAdded
	case 'D':
		return gitdomain.GitFileStatusDeleted
	case 'R', 'C':
		return gitdomain.GitFileStatusRenamed
	default:
		return gitdomain.GitFileStatusModified
	}
}

func parseCount(
	s string,
) int {
	if s == "-" {
		return -1
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}
