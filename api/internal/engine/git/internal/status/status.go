// Package status parses git status --porcelain=v2 --branch output into domain types.
package status

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// Parse runs git status in repoPath and returns the parsed working-tree status.
func Parse(
	ctx context.Context,
	repoPath string,
) (gitdomain.GitStatus, error) {
	r := exec.Git(ctx, repoPath, "status", "--porcelain=v2", "--branch")
	if r.ExitCode != 0 {
		return gitdomain.GitStatus{}, fmt.Errorf("status: parse: exit %d: %s", r.ExitCode, strings.TrimSpace(r.Stderr))
	}
	return parseOutput(r.Stdout), nil
}

func parseOutput(
	output string,
) gitdomain.GitStatus {
	var s gitdomain.GitStatus
	lines := strings.Split(output, "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		switch {
		case strings.HasPrefix(line, "# branch.head "):
			s.Branch = strings.TrimPrefix(line, "# branch.head ")
		case strings.HasPrefix(line, "# branch.ab "):
			s.Ahead, s.Behind = parseAheadBehind(strings.TrimPrefix(line, "# branch.ab "))
		case strings.HasPrefix(line, "1 "):
			s.Files = append(s.Files, parseOrdinaryEntries(line)...)
		case strings.HasPrefix(line, "2 "):
			s.Files = append(s.Files, parseRenamedEntry(line))
		case strings.HasPrefix(line, "u "):
			s.Files = append(s.Files, parseUnmergedEntry(line))
		case strings.HasPrefix(line, "? "):
			s.Files = append(s.Files, parseUntrackedEntry(line))
		}
	}
	return s
}

func parseAheadBehind(
	ab string,
) (int, int) {
	parts := strings.Fields(ab)
	if len(parts) != 2 {
		return 0, 0
	}
	ahead, _ := strconv.Atoi(strings.TrimPrefix(parts[0], "+"))
	behind, _ := strconv.Atoi(strings.TrimPrefix(parts[1], "-"))
	return ahead, behind
}

func parseOrdinaryEntries(
	line string,
) []gitdomain.GitFile {
	parts := strings.Fields(line)
	if len(parts) < 9 {
		return nil
	}
	xy := parts[1]
	if len(xy) < 2 {
		return nil
	}
	x := rune(xy[0])
	y := rune(xy[1])
	path := parts[8]

	if isConflict(xy) {
		return []gitdomain.GitFile{
			{Path: path, Status: gitdomain.GitFileStatusConflicted, Staged: false},
		}
	}

	var files []gitdomain.GitFile
	if x != '.' {
		files = append(files, gitdomain.GitFile{
			Path:   path,
			Status: charToStatus(x),
			Staged: true,
		})
	}
	if y != '.' {
		files = append(files, gitdomain.GitFile{
			Path:   path,
			Status: charToStatus(y),
			Staged: false,
		})
	}
	return files
}

func parseRenamedEntry(
	line string,
) gitdomain.GitFile {
	parts := strings.Fields(line)
	if len(parts) < 10 {
		return gitdomain.GitFile{}
	}
	xy := parts[1]
	if len(xy) < 2 {
		return gitdomain.GitFile{}
	}
	x := rune(xy[0])
	path := parts[9]
	staged := x == 'R' || x == 'C'
	return gitdomain.GitFile{
		Path:   path,
		Status: gitdomain.GitFileStatusRenamed,
		Staged: staged,
	}
}

func parseUnmergedEntry(
	line string,
) gitdomain.GitFile {
	parts := strings.Fields(line)
	if len(parts) < 11 {
		return gitdomain.GitFile{}
	}
	path := parts[10]
	return gitdomain.GitFile{
		Path:   path,
		Status: gitdomain.GitFileStatusConflicted,
		Staged: false,
	}
}

func parseUntrackedEntry(
	line string,
) gitdomain.GitFile {
	path := strings.TrimPrefix(line, "? ")
	return gitdomain.GitFile{
		Path:   path,
		Status: gitdomain.GitFileStatusUntracked,
		Staged: false,
	}
}

func isConflict(
	xy string,
) bool {
	if len(xy) < 2 {
		return false
	}
	x := xy[0]
	y := xy[1]
	if y == 'U' || x == 'U' {
		return true
	}
	if xy == "AA" || xy == "DD" {
		return true
	}
	return false
}

func charToStatus(
	c rune,
) gitdomain.GitFileStatus {
	switch c {
	case 'M':
		return gitdomain.GitFileStatusModified
	case 'A':
		return gitdomain.GitFileStatusAdded
	case 'D':
		return gitdomain.GitFileStatusDeleted
	case 'R':
		return gitdomain.GitFileStatusRenamed
	default:
		return gitdomain.GitFileStatusModified
	}
}
