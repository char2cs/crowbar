package worktree

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// maxBranchNameAttempts bounds how many candidates generateBranchName tries
// before giving up. A collision needs two independently-drawn UUID prefixes to
// match, so this is generous headroom against an adversarial ref list, not an
// expected retry count.
const maxBranchNameAttempts = 20

// generateBranchName mints a provisional branch name for a spontaneous
// worktree create (model spec §4.1: "generated server-side... the row is
// marked provisional until renamed"), collision-checked against every REAL ref
// the repo already has — local and remote alike, which is exactly what a
// client cannot see and the reason this has to happen here.
//
// CreateChild calls it only when the caller left Branch blank; every existing
// explicit-create caller keeps supplying its own name and never reaches this.
func (u *worktreeUsecase) generateBranchName(
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

// branchNameCandidate draws one candidate: a fixed prefix plus a short random
// suffix, filesystem-safe as a single path segment since a branch name becomes
// the leaf directory of its workspace root (deriveWorktreePath).
//
// A package-level var, not a plain call, so a test can pin what
// generateBranchName tries — see export_test.go, mirroring the same pattern
// the git engine's own branches.go uses for its gitRunner seam.
var branchNameCandidate = provisionalBranchName

func provisionalBranchName() string {
	return "chat-" + strings.SplitN(uuid.NewString(), "-", 2)[0]
}

// collides reports whether candidate matches a real ref name, local or
// remote. `git branch -a` lists a remote ref as "<remote>/<name>" — comparing
// only the exact string would miss "origin/chat-XXXXXXXX" colliding with the
// local name "chat-XXXXXXXX" it tracks.
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
