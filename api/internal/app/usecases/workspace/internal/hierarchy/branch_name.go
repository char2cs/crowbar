package hierarchy

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// maxBranchNameAttempts bounds generateBranchName's tries: generous headroom against an
// adversarial ref list, since a collision needs two random UUID prefixes to match.
const maxBranchNameAttempts = 20

// generateBranchName mints a provisional branch name when CreateChild gets a blank
// Branch, checked against every local and remote ref: refs a client cannot see, which
// is why it happens server-side.
func (u *hierarchyUsecase) generateBranchName(
	ctx context.Context,
	repoPath string,
) (string, error) {
	existing, err := u.git.Branches(ctx, repoPath)
	if err != nil {
		return "", fmt.Errorf("generate branch name: list branches: %w", err)
	}
	taken := make(map[string]bool, len(existing))
	for _, b := range existing {
		taken[b.Name] = true
	}
	for range maxBranchNameAttempts {
		candidate := branchNameCandidate()
		if !collides(taken, candidate) {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("generate branch name: no free name after %d attempts", maxBranchNameAttempts)
}

// branchNameCandidate draws a fixed prefix plus a short random suffix, safe as one path
// segment since the branch names its workspace root's leaf directory. It is a var so
// export_test.go can pin the candidates.
var branchNameCandidate = provisionalBranchName

func provisionalBranchName() string {
	return "chat-" + strings.SplitN(uuid.NewString(), "-", 2)[0]
}

// collides reports whether candidate matches a local or remote ref. Remote refs list
// as "<remote>/<name>", so an exact match alone would miss "origin/<candidate>".
func collides(
	taken map[string]bool,
	candidate string,
) bool {
	if taken[candidate] {
		return true
	}
	for name := range taken {
		if strings.HasSuffix(name, "/"+candidate) {
			return true
		}
	}
	return false
}
