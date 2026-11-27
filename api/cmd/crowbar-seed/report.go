package main

import (
	"fmt"
	"io"
	"strings"
)

type report struct {
	out       io.Writer
	lines     []string
	project   projectDTO
	repo      repoDTO
	workspace chatDTO
}

func (r *report) note(
	created bool,
	what string,
) {
	verb := "reused"
	if created {
		verb = "created"
	}
	r.noteVerb(verb, what)
}

func (r *report) noteRepair(
	what string,
) {
	r.noteVerb("repaired", what)
}

func (r *report) noteVerb(
	verb string,
	what string,
) {
	r.lines = append(r.lines, fmt.Sprintf("  %-8s  %s", verb, what))
}

func (r *report) noteThreads(
	created int,
) {
	total := len(seedThreads())
	if created == 0 {
		r.note(false, fmt.Sprintf("%d review threads", total))
		return
	}
	if created == total {
		r.note(true, fmt.Sprintf("%d review threads", total))
		return
	}
	r.note(true, fmt.Sprintf("%d review threads (%d reused)", created, total-created))
}

func (r *report) write() error {
	route := fmt.Sprintf("/ide/%s/%s/%s", r.project.ID, r.repo.ID, r.workspace.WorkspaceID)
	_, err := fmt.Fprintf(r.out, `
Crowbar dev instance seeded.

%s

  project    %s (%s)
  repo       %s (%s)
  workspace  %s (%s)
  branch     %s
  worktree   %s

  Open      %s
            http://localhost:5173%s  (make dev-web / dev-desktop)
`,
		strings.Join(r.lines, "\n"),
		seedProjectName, r.project.ID,
		r.repo.Name, r.repo.ID,
		seedFeatureBranch, r.workspace.WorkspaceID,
		r.workspace.Worktree.Branch,
		r.workspace.Worktree.LocalPath,
		route, route,
	)
	if err != nil {
		return fmt.Errorf("seed: write summary: %w", err)
	}
	return nil
}
