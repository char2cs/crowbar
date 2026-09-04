package main

import (
	"strings"
	"testing"
)

func TestReportDistinguishesCreatedFromReused(t *testing.T) {
	rep := &report{}
	rep.note(true, "project Crowbar Seed")
	rep.note(false, "repo checkout on main")
	rep.noteRepair("worktree for feature/pricing-rounding")

	joined := strings.Join(rep.lines, "\n")
	for _, want := range []string{
		"created   project Crowbar Seed",
		"reused    repo checkout on main",
		"repaired  worktree for feature/pricing-rounding",
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %q:\n%s", want, joined)
		}
	}
}

func TestReportThreadsReadsAsReusedWhenNothingWasOpened(t *testing.T) {
	rep := &report{}
	rep.noteThreads(0)

	if !strings.Contains(rep.lines[0], "reused") {
		t.Fatalf("line = %q", rep.lines[0])
	}
}

// A rerun that opens one of the two threads is exactly the case a bare count
// would hide, so the summary has to name both halves.
func TestReportThreadsSpellsOutThePartialCase(t *testing.T) {
	rep := &report{}
	rep.noteThreads(1)

	if !strings.Contains(rep.lines[0], "created") || !strings.Contains(rep.lines[0], "1 reused") {
		t.Fatalf("line = %q", rep.lines[0])
	}
}

func TestReportWriteNamesTheWorkspaceAndItsWorktree(t *testing.T) {
	out := &strings.Builder{}
	rep := &report{
		out:     out,
		project: projectDTO{ID: "p1"},
		repo:    repoDTO{ID: "r1", Name: seedRepoName},
		workspace: chatDTO{
			ID:          "chat1",
			WorkspaceID: "w1",
			Worktree:    &chatWorktreeDTO{Branch: seedFeatureBranch, LocalPath: "/wt/seed"},
		},
	}

	if err := rep.write(); err != nil {
		t.Fatalf("write: %v", err)
	}
	for _, want := range []string{seedFeatureBranch, "/wt/seed", "/ide/p1/r1/w1"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("summary is missing %q:\n%s", want, out)
		}
	}
}
