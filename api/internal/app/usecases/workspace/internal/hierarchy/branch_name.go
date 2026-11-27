package hierarchy

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

const maxBranchNameAttempts = 20

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

var branchNameCandidate = provisionalBranchName

func provisionalBranchName() string {
	return "chat-" + strings.SplitN(uuid.NewString(), "-", 2)[0]
}

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
