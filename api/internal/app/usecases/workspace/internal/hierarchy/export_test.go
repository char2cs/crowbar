package hierarchy

func SetBranchNameCandidateForTest(fn func() string) (restore func()) {
	old := branchNameCandidate
	branchNameCandidate = fn
	return func() { branchNameCandidate = old }
}
