package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEnsureFixtureBuildsRepoOnBaseBranch(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), seedRepoName)

	repo, created, err := ensureFixture(repoPath)
	if err != nil {
		t.Fatalf("ensureFixture: %v", err)
	}
	if !created {
		t.Fatal("expected the first ensureFixture to report the repo as created")
	}

	branch, err := repo.output("rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		t.Fatalf("rev-parse: %v", err)
	}
	if strings.TrimSpace(branch) != seedBaseBranch {
		t.Fatalf("branch = %q, want %q", strings.TrimSpace(branch), seedBaseBranch)
	}
}

func TestEnsureFixtureCommitsEachChangeSeparately(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), seedRepoName)
	repo, _, err := ensureFixture(repoPath)
	if err != nil {
		t.Fatalf("ensureFixture: %v", err)
	}

	log, err := repo.output("log", "--pretty=%s")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	subjects := strings.Split(strings.TrimSpace(log), "\n")
	if len(subjects) != len(fixtureCommits()) {
		t.Fatalf("got %d commits, want %d: %q", len(subjects), len(fixtureCommits()), subjects)
	}
	if subjects[len(subjects)-1] != fixtureCommits()[0].subject {
		t.Fatalf("oldest commit = %q, want %q", subjects[len(subjects)-1], fixtureCommits()[0].subject)
	}
}

func TestEnsureFixtureWritesEveryTrackedSource(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), seedRepoName)
	repo, _, err := ensureFixture(repoPath)
	if err != nil {
		t.Fatalf("ensureFixture: %v", err)
	}

	tracked, err := repo.output("ls-files")
	if err != nil {
		t.Fatalf("ls-files: %v", err)
	}
	for _, commit := range fixtureCommits() {
		for _, file := range commit.files {
			if !strings.Contains(tracked, file.path) {
				t.Fatalf("%s is not tracked; ls-files = %q", file.path, tracked)
			}
		}
	}
}

// The fixture only earns its keep if the review surface has a real defect to
// render, so the generated pricing source must carry the seeded bugs verbatim.
func TestFixtureSourceCarriesTheReviewableDefects(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), seedRepoName)
	if _, _, err := ensureFixture(repoPath); err != nil {
		t.Fatalf("ensureFixture: %v", err)
	}

	body, err := os.ReadFile(filepath.Join(repoPath, pricingPath))
	if err != nil {
		t.Fatalf("read pricing source: %v", err)
	}
	for _, defect := range []string{"i <= items.length", "subtotal(items) / items.length"} {
		if !strings.Contains(string(body), defect) {
			t.Fatalf("pricing source is missing the %q defect", defect)
		}
	}
}

func TestEnsureFixtureReusesAnExistingRepo(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), seedRepoName)
	if _, _, err := ensureFixture(repoPath); err != nil {
		t.Fatalf("first ensureFixture: %v", err)
	}
	marker := filepath.Join(repoPath, "LOCAL.md")
	if err := os.WriteFile(marker, []byte("edited by hand\n"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	_, created, err := ensureFixture(repoPath)
	if err != nil {
		t.Fatalf("second ensureFixture: %v", err)
	}
	if created {
		t.Fatal("second ensureFixture reported a create; it must reuse the existing repo")
	}
	if _, err := os.Stat(marker); err != nil {
		t.Fatalf("reuse clobbered local state: %v", err)
	}
}

func TestEnsureBranchDiffCommitsTheFollowUpChange(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), seedRepoName)
	repo, _, err := ensureFixture(repoPath)
	if err != nil {
		t.Fatalf("ensureFixture: %v", err)
	}

	created, err := ensureBranchDiff(repo)
	if err != nil {
		t.Fatalf("ensureBranchDiff: %v", err)
	}
	if !created {
		t.Fatal("expected ensureBranchDiff to report a new commit")
	}

	changed, err := repo.output("diff", "--name-only", "HEAD~1", "HEAD")
	if err != nil {
		t.Fatalf("diff: %v", err)
	}
	for _, file := range branchFiles() {
		if !strings.Contains(changed, file.path) {
			t.Fatalf("%s is not in the review commit; changed = %q", file.path, changed)
		}
	}
}

func TestEnsureBranchDiffIsIdempotent(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), seedRepoName)
	repo, _, err := ensureFixture(repoPath)
	if err != nil {
		t.Fatalf("ensureFixture: %v", err)
	}
	if _, err := ensureBranchDiff(repo); err != nil {
		t.Fatalf("first ensureBranchDiff: %v", err)
	}

	created, err := ensureBranchDiff(repo)
	if err != nil {
		t.Fatalf("second ensureBranchDiff: %v", err)
	}
	if created {
		t.Fatal("second ensureBranchDiff committed again; it must recognise its own commit")
	}

	log, err := repo.output("log", "--pretty=%s")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if strings.Count(log, branchCommitSubject) != 1 {
		t.Fatalf("review commit appears %d times in %q", strings.Count(log, branchCommitSubject), log)
	}
}

func TestLineOfResolvesReviewAnchors(t *testing.T) {
	for _, thread := range seedThreads() {
		line, err := lineOf(branchPricingSource, thread.anchor)
		if err != nil {
			t.Fatalf("anchor %q: %v", thread.anchor, err)
		}
		if line <= 0 {
			t.Fatalf("anchor %q resolved to line %d", thread.anchor, line)
		}
	}
}

// Every seeded thread must land on a line the review commit actually changed,
// or the comment renders detached from the code it is about.
func TestSeedThreadAnchorsAreOnChangedLines(t *testing.T) {
	for _, thread := range seedThreads() {
		if strings.Contains(mainPricingSource, thread.anchor) {
			t.Fatalf("anchor %q is unchanged by the review commit", thread.anchor)
		}
	}
}

func TestLineOfRejectsAMissingAnchor(t *testing.T) {
	if _, err := lineOf(branchPricingSource, "no such line anywhere"); err == nil {
		t.Fatal("expected a missing anchor to be an error")
	}
}
