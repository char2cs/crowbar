package worktree

// SetBranchNameCandidateForTest overrides the provisional-branch-name
// candidate source generateBranchName draws from, so a test can pin what it
// tries (and prove the retry-on-collision behaviour) without depending on a
// real UUID draw. Returns a restore function.
func SetBranchNameCandidateForTest(fn func() string) (restore func()) {
	old := branchNameCandidate
	branchNameCandidate = fn
	return func() { branchNameCandidate = old }
}
