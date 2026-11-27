package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func enclosingRepo(
	t *testing.T,
) (string, string) {
	t.Helper()
	outer := t.TempDir()
	for _, args := range [][]string{
		{"init", "--quiet"},
		{"config", "user.name", fixtureAuthorName},
		{"config", "user.email", fixtureAuthorEmail},
		{"commit", "--quiet", "--allow-empty", "-m", "enclosing repository"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = outer
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
	}
	inner := filepath.Join(outer, "state", "not-a-repo")
	if err := os.MkdirAll(inner, 0o750); err != nil {
		t.Fatalf("mkdir inner: %v", err)
	}
	return outer, inner
}

func TestFenceEnvStopsGitFromWalkingUpOutOfTheDirectory(t *testing.T) {
	outer, inner := enclosingRepo(t)

	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = inner
	cmd.Env = fenceEnv(inner)
	out, err := cmd.CombinedOutput()
	if err == nil {
		t.Fatalf("git escaped the fence and resolved %q; the enclosing repo is %s", out, outer)
	}
	if strings.Contains(string(out), outer) {
		t.Fatalf("git reported the enclosing repository %s: %s", outer, out)
	}
}

func TestVerifyToplevelRefusesADirectoryThatResolvesToAnEnclosingRepository(t *testing.T) {
	outer, inner := enclosingRepo(t)
	resolved, err := realPath(inner)
	if err != nil {
		t.Fatalf("realPath: %v", err)
	}

	err = verifyToplevel(resolved, os.Environ())
	if err == nil {
		t.Fatal("verifyToplevel accepted a directory whose git commands land in the enclosing repository")
	}
	if !strings.Contains(err.Error(), "refusing") {
		t.Fatalf("error does not name the refusal: %v", err)
	}
	top, topErr := realPath(outer)
	if topErr != nil {
		t.Fatalf("realPath outer: %v", topErr)
	}
	if !strings.Contains(err.Error(), top) {
		t.Fatalf("error does not name the repository git actually resolved (%s): %v", top, err)
	}
}

func TestFenceEnvDropsInheritedGitOverrides(t *testing.T) {
	outer, inner := enclosingRepo(t)
	t.Setenv("GIT_DIR", filepath.Join(outer, ".git"))
	t.Setenv("GIT_CEILING_DIRECTORIES", "/")

	env := fenceEnv(inner)
	for _, entry := range env {
		name, _, _ := strings.Cut(entry, "=")
		if name == "GIT_DIR" {
			t.Fatalf("GIT_DIR survived into the fenced environment: %q", entry)
		}
	}
	if got := ceilingOf(t, env); got != filepath.Dir(inner) {
		t.Fatalf("ceiling = %q, want %q", got, filepath.Dir(inner))
	}
}

func ceilingOf(
	t *testing.T,
	env []string,
) string {
	t.Helper()
	ceiling := ""
	seen := 0
	for _, entry := range env {
		name, value, _ := strings.Cut(entry, "=")
		if name == "GIT_CEILING_DIRECTORIES" {
			ceiling = value
			seen++
		}
	}
	if seen != 1 {
		t.Fatalf("expected exactly one GIT_CEILING_DIRECTORIES, got %d", seen)
	}
	return ceiling
}

func TestOpenRepoRefusesADirectoryNestedInAnotherRepository(t *testing.T) {
	outer, inner := enclosingRepo(t)

	repo, err := openRepo(inner)
	if err == nil {
		t.Fatalf("openRepo handed back an invoker rooted at %s inside %s", repo.root, outer)
	}
}

func TestOpenRepoAcceptsTheRepositoryItIsPointedAt(t *testing.T) {
	repoPath := filepath.Join(t.TempDir(), seedRepoName)
	if _, _, err := ensureFixture(repoPath); err != nil {
		t.Fatalf("ensureFixture: %v", err)
	}

	repo, err := openRepo(repoPath)
	if err != nil {
		t.Fatalf("openRepo: %v", err)
	}
	want, err := realPath(repoPath)
	if err != nil {
		t.Fatalf("realPath: %v", err)
	}
	if repo.root != want {
		t.Fatalf("root = %q, want %q", repo.root, want)
	}
}

func TestEnsureFixtureInsideAnotherRepositoryStaysItsOwnRepository(t *testing.T) {
	outer, _ := enclosingRepo(t)
	repoPath := filepath.Join(outer, ".crowbar", "seed", seedRepoName)

	fixture, created, err := ensureFixture(repoPath)
	if err != nil {
		t.Fatalf("ensureFixture: %v", err)
	}
	if !created {
		t.Fatal("expected the fixture to be created")
	}
	want, err := realPath(repoPath)
	if err != nil {
		t.Fatalf("realPath: %v", err)
	}
	if fixture.root != want {
		t.Fatalf("fixture root = %q, want %q", fixture.root, want)
	}

	cmd := exec.Command("git", "log", "--pretty=%s")
	cmd.Dir = outer
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("log the enclosing repository: %v: %s", err, out)
	}
	if strings.Contains(string(out), fixtureCommits()[0].subject) {
		t.Fatalf("the seed committed into the enclosing repository:\n%s", out)
	}
}
