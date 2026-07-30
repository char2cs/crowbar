package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// seedBaseBranch is the fixture's default branch. It is created explicitly
// rather than left to init.defaultBranch, which varies per developer and would
// make the seeded repo's base branch unpredictable.
const seedBaseBranch = "main"

// The fixture pins its own committer, repo-locally. A developer with no global
// user.name still gets a working fixture, and one who has it keeps it untouched.
const (
	fixtureAuthorName  = "Crowbar Seed"
	fixtureAuthorEmail = "seed@crowbar.local"
)

// branchCommitSubject doubles as the idempotency marker: a rerun that finds it
// at the feature worktree's HEAD leaves the existing commit alone.
const branchCommitSubject = "fix(pricing): guard the empty cart and round to whole cents"

const branchCommitBody = `subtotal() walked one index past the end of the items array, and
averageLineValue() divided by a zero-length cart. Both are fixed here, and
every rate multiplication now lands back on an integral cent through
roundToCents().`

type sourceFile struct {
	path    string
	content string
}

type fixtureCommit struct {
	subject string
	files   []sourceFile
}

// fixtureCommits is the history the generated repo starts with: a few real
// commits so `git log`, blame and the commit picker all have something to show.
func fixtureCommits() []fixtureCommit {
	return []fixtureCommit{
		{
			subject: "chore: scaffold the checkout service",
			files:   []sourceFile{{path: "README.md", content: readmeSource}},
		},
		{
			subject: "feat(pricing): line-item subtotals, bulk discounts and tax",
			files:   []sourceFile{{path: "src/pricing.ts", content: mainPricingSource}},
		},
		{
			subject: "feat(inventory): reserve stock against the warehouse ledger",
			files:   []sourceFile{{path: "src/inventory.ts", content: inventorySource}},
		},
	}
}

func branchFiles() []sourceFile {
	return []sourceFile{
		{path: "src/pricing.ts", content: branchPricingSource},
		{path: "src/pricing.test.ts", content: branchPricingTestSource},
	}
}

// ensureFixture generates the throwaway repo the seed imports, and reports
// whether it had to create it. An existing repo is reused untouched: once
// Crowbar has imported it, its worktrees and branches are live state that
// regenerating would strand.
func ensureFixture(
	repoPath string,
) (bool, error) {
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		return false, nil
	}
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		return false, fmt.Errorf("seed: create fixture dir: %w", err)
	}
	if err := initFixtureRepo(repoPath); err != nil {
		return false, err
	}
	for _, commit := range fixtureCommits() {
		if err := applyFixtureCommit(repoPath, commit); err != nil {
			return false, err
		}
	}
	return true, nil
}

func initFixtureRepo(
	repoPath string,
) error {
	if err := git(repoPath, "init", "--quiet"); err != nil {
		return err
	}
	if err := git(repoPath, "symbolic-ref", "HEAD", "refs/heads/"+seedBaseBranch); err != nil {
		return err
	}
	if err := git(repoPath, "config", "user.name", fixtureAuthorName); err != nil {
		return err
	}
	return git(repoPath, "config", "user.email", fixtureAuthorEmail)
}

func applyFixtureCommit(
	repoPath string,
	commit fixtureCommit,
) error {
	if err := writeSources(repoPath, commit.files); err != nil {
		return err
	}
	if err := git(repoPath, "add", "-A"); err != nil {
		return err
	}
	return git(repoPath, "commit", "--quiet", "-m", commit.subject)
}

// ensureBranchDiff commits the follow-up change inside the feature workspace's
// own worktree. Without it the workspace is an empty branch and the review and
// diff panes — the surfaces this seed exists to populate — render nothing.
func ensureBranchDiff(
	worktreePath string,
) (bool, error) {
	head, err := gitOutput(worktreePath, "log", "-1", "--pretty=%s")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(head) == branchCommitSubject {
		return false, nil
	}
	if err := writeSources(worktreePath, branchFiles()); err != nil {
		return false, err
	}
	if err := git(worktreePath, "add", "-A"); err != nil {
		return false, err
	}
	message := branchCommitSubject + "\n\n" + branchCommitBody
	if err := git(worktreePath, "commit", "--quiet", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

// lineOf resolves a review anchor to a line number in the branch source, so the
// seeded threads stay pinned to the code they talk about when the fixture text
// is edited. Anchoring by hardcoded line number rots on the first edit.
func lineOf(
	content string,
	anchor string,
) (int, error) {
	for i, line := range strings.Split(content, "\n") {
		if strings.Contains(line, anchor) {
			return i + 1, nil
		}
	}
	return 0, fmt.Errorf("seed: review anchor %q is not in the branch source", anchor)
}
