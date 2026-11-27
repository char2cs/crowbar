package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func seededWorktree(
	t *testing.T,
) (*gitRepo, string) {
	t.Helper()
	home := t.TempDir()
	fixture, _, err := ensureFixture(filepath.Join(home, "seed", seedRepoName))
	if err != nil {
		t.Fatalf("ensureFixture: %v", err)
	}
	path := filepath.Join(home, "projects", "checkout", "feature", "worktree")
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := addWorktree(fixture, path, seedFeatureBranch); err != nil {
		t.Fatalf("addWorktree: %v", err)
	}
	return fixture, path
}

func TestEnsureWorktreeReusesAHealthyWorktree(t *testing.T) {
	fixture, path := seededWorktree(t)

	worktree, repaired, err := ensureWorktree(fixture, path, seedFeatureBranch)
	if err != nil {
		t.Fatalf("ensureWorktree: %v", err)
	}
	if repaired {
		t.Fatal("a healthy worktree was reported as repaired")
	}
	if _, err := worktree.output("log", "-1", "--pretty=%s"); err != nil {
		t.Fatalf("log in the reused worktree: %v", err)
	}
}

func TestEnsureWorktreeRepairsAVanishedRegistration(t *testing.T) {
	fixture, path := seededWorktree(t)
	registry, err := fixture.commonDir()
	if err != nil {
		t.Fatalf("commonDir: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(registry, worktreeRegistryDir)); err != nil {
		t.Fatalf("drop the registration: %v", err)
	}
	if _, err := runGit(path, "log", "-1", "--pretty=%s"); err == nil {
		t.Fatal("the setup did not actually break the worktree")
	}

	worktree, repaired, err := ensureWorktree(fixture, path, seedFeatureBranch)
	if err != nil {
		t.Fatalf("ensureWorktree: %v", err)
	}
	if !repaired {
		t.Fatal("a dangling worktree was not reported as repaired")
	}
	if _, err := worktree.output("log", "-1", "--pretty=%s"); err != nil {
		t.Fatalf("log in the repaired worktree: %v", err)
	}
}

func TestEnsureWorktreeRepairsAVanishedDirectory(t *testing.T) {
	fixture, path := seededWorktree(t)
	if err := os.RemoveAll(path); err != nil {
		t.Fatalf("drop the directory: %v", err)
	}

	worktree, repaired, err := ensureWorktree(fixture, path, seedFeatureBranch)
	if err != nil {
		t.Fatalf("ensureWorktree: %v", err)
	}
	if !repaired {
		t.Fatal("a vanished worktree directory was not reported as repaired")
	}
	if _, err := worktree.output("log", "-1", "--pretty=%s"); err != nil {
		t.Fatalf("log in the repaired worktree: %v", err)
	}
}

func TestRepairKeepsTheBranchHistoryTheFixtureStillHas(t *testing.T) {
	fixture, path := seededWorktree(t)
	worktree, err := openWorktree(fixture, path)
	if err != nil {
		t.Fatalf("openWorktree: %v", err)
	}
	if _, err := ensureBranchDiff(worktree); err != nil {
		t.Fatalf("ensureBranchDiff: %v", err)
	}
	registry, err := fixture.commonDir()
	if err != nil {
		t.Fatalf("commonDir: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(registry, worktreeRegistryDir)); err != nil {
		t.Fatalf("drop the registration: %v", err)
	}

	repaired, _, err := ensureWorktree(fixture, path, seedFeatureBranch)
	if err != nil {
		t.Fatalf("ensureWorktree: %v", err)
	}
	head, err := repaired.output("log", "-1", "--pretty=%s")
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	if strings.TrimSpace(head) != branchCommitSubject {
		t.Fatalf("head = %q, want the review commit %q", strings.TrimSpace(head), branchCommitSubject)
	}
}

func TestEnsureWorktreeRefusesAPathTheFixtureDoesNotOwn(t *testing.T) {
	fixture, _ := seededWorktree(t)
	outer, inner := enclosingRepo(t)
	keep := filepath.Join(inner, "precious.txt")
	if err := os.WriteFile(keep, []byte("not the seed's to delete\n"), 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := ensureWorktree(fixture, inner, seedFeatureBranch)
	if err == nil {
		t.Fatalf("ensureWorktree adopted a directory inside the unrelated repository at %s", outer)
	}
	if _, statErr := os.Stat(keep); statErr != nil {
		t.Fatalf("the refused directory was deleted anyway: %v", statErr)
	}
}

func TestOpenWorktreeRefusesAWorktreeOfAnotherRepository(t *testing.T) {
	fixture, _ := seededWorktree(t)
	other, otherPath := seededWorktree(t)

	_, err := openWorktree(fixture, otherPath)
	if err == nil {
		t.Fatalf("openWorktree accepted a worktree of %s", other.root)
	}
	if !strings.Contains(err.Error(), "not of the fixture") {
		t.Fatalf("error does not name the mismatch: %v", err)
	}
}

func TestOrphanedOfIgnoresAWorktreeWhoseRegistrationIsIntact(t *testing.T) {
	fixture, path := seededWorktree(t)

	orphaned, err := orphanedOf(fixture, path)
	if err != nil {
		t.Fatalf("orphanedOf: %v", err)
	}
	if orphaned {
		t.Fatal("a live worktree was reported as orphaned; clearOrphan would have deleted it")
	}
}

func TestWorktreeGitdirIgnoresARealGitDirectory(t *testing.T) {
	fixture, _ := seededWorktree(t)

	target, err := worktreeGitdir(fixture.root)
	if err != nil {
		t.Fatalf("worktreeGitdir: %v", err)
	}
	if target != "" {
		t.Fatalf("target = %q, want empty for a main worktree", target)
	}
}
