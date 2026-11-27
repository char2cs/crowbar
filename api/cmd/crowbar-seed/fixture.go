package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const seedBaseBranch = "main"

const (
	fixtureAuthorName  = "Crowbar Seed"
	fixtureAuthorEmail = "seed@crowbar.local"
)

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
