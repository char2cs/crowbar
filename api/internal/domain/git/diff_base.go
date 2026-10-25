package git

// DiffBase is the commit a working-tree summary diffs against, plus whether the
// branch has commits beyond it. Commit is empty when no base resolves.
type DiffBase struct {
	Commit     string
	HasCommits bool
}
