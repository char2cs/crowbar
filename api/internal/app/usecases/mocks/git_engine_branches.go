package mocks

import "context"

// RemoteTrackingBranchExists answers from RemoteTrackingBranches (default false).
func (g *GitEngine) RemoteTrackingBranchExists(_ context.Context, _, branch string) (bool, error) {
	return g.RemoteTrackingBranches[branch], nil
}

// LocalBranchExists answers from MissingLocalBranches (default: every branch exists).
func (g *GitEngine) LocalBranchExists(_ context.Context, _, branch string) (bool, error) {
	return !g.MissingLocalBranches[branch], nil
}
