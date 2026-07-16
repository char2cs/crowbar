package diff

import (
	"context"
	"strconv"
	"strings"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// FileSummaries returns the files-only diff of the working tree against ref —
// committed changes since ref merged with uncommitted tracked modifications,
// exactly the file set of AgainstRef — but WITHOUT any hunk content. It runs
// `git diff --name-status -M -z <ref> --` for the per-file status classification
// and `git diff --numstat -M -z <ref> --` for the +/- line counts, then joins
// them by path. Both are O(file count) in output size, never O(diff size).
//
// Untracked files are not included (they have no diff against ref); the caller
// folds them in from git status. Additions/Deletions are -1 for binary files
// (numstat "-"). Renames carry the NEW path in Path and the source in OldPath.
func FileSummaries(
	ctx context.Context,
	repoPath string,
	ref string,
) ([]gitdomain.ReviewFileSummary, error) {
	nameStatus := exec.Git(ctx, repoPath, "diff", "--name-status", "-M", "-z", ref, "--")
	if err := exec.RequireSuccess("diff: file summary name-status", nameStatus); err != nil {
		return nil, err
	}
	numstat := exec.Git(ctx, repoPath, "diff", "--numstat", "-M", "-z", ref, "--")
	if err := exec.RequireSuccess("diff: file summary numstat", numstat); err != nil {
		return nil, err
	}
	entries := parseNameStatusZ(nameStatus.Stdout)
	counts := parseNumstatZ(numstat.Stdout)
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
