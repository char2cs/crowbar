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
) (*gitRepo, bool, error) {
	if _, err := os.Stat(filepath.Join(repoPath, ".git")); err == nil {
		repo, err := openRepo(repoPath)
		return repo, false, err
	}
	if err := os.MkdirAll(repoPath, 0o750); err != nil {
		return nil, false, fmt.Errorf("seed: create fixture dir: %w", err)
	}
	repo, err := initFixtureRepo(repoPath)
	if err != nil {
		return nil, false, err
	}
	for _, commit := range fixtureCommits() {
		if err := applyFixtureCommit(repo, commit); err != nil {
			return nil, false, err
		}
	}
	return repo, true, nil
}

func initFixtureRepo(
	repoPath string,
) (*gitRepo, error) {
	if _, err := runGit(repoPath, "init", "--quiet"); err != nil {
		return nil, err
	}
	repo, err := openRepo(repoPath)
	if err != nil {
		return nil, err
	}
	if err := repo.run("symbolic-ref", "HEAD", "refs/heads/"+seedBaseBranch); err != nil {
		return nil, err
	}
	if err := repo.run("config", "user.name", fixtureAuthorName); err != nil {
		return nil, err
	}
	if err := repo.run("config", "user.email", fixtureAuthorEmail); err != nil {
		return nil, err
	}
	return repo, nil
}

func applyFixtureCommit(
	repo *gitRepo,
	commit fixtureCommit,
) error {
	if err := writeSources(repo.root, commit.files); err != nil {
		return err
	}
	if err := repo.run("add", "-A"); err != nil {
		return err
	}
	return repo.run("commit", "--quiet", "-m", commit.subject)
}

// ensureBranchDiff commits the follow-up change inside the feature workspace's
// own worktree. Without it the workspace is an empty branch and the review and
// diff panes — the surfaces this seed exists to populate — render nothing.
func ensureBranchDiff(
	worktree *gitRepo,
) (bool, error) {
	head, err := worktree.output("log", "-1", "--pretty=%s")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(head) == branchCommitSubject {
		return false, nil
	}
	if err := writeSources(worktree.root, branchFiles()); err != nil {
		return false, err
	}
	if err := worktree.run("add", "-A"); err != nil {
		return false, err
	}
	message := branchCommitSubject + "\n\n" + branchCommitBody
	if err := worktree.run("commit", "--quiet", "-m", message); err != nil {
		return false, err
	}
	return true, nil
}

func writeSources(
	dir string,
	files []sourceFile,
) error {
	for _, file := range files {
		if err := writeSource(dir, file); err != nil {
			return err
		}
	}
	return nil
}

func writeSource(
	dir string,
	file sourceFile,
) error {
	target := filepath.Join(dir, file.path)
	if err := os.MkdirAll(filepath.Dir(target), 0o750); err != nil {
		return fmt.Errorf("seed: create %s: %w", filepath.Dir(file.path), err)
	}
	if err := os.WriteFile(target, []byte(file.content), 0o644); err != nil { //nolint:gosec // G306: throwaway fixture sources, not secrets
		return fmt.Errorf("seed: write %s: %w", file.path, err)
	}
	return nil
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
