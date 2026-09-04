package main

import (
	"fmt"
	"io"
	"strings"
)

// report accumulates what the run did so the closing summary can say, per
// entity, whether it was created now or reused from an earlier run — the one
// thing a developer re-running the seed actually wants to know.
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

// noteRepair is its own verb because a repair is neither of the two outcomes a
// rerun expects: it means the run found the fixture and the daemon's idea of it
// out of step and put them back together, which is worth seeing.
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

// noteThreads spells the partial case out. A rerun that finds one thread and
// opens the other is the case a bare "created 1" would hide.
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
	// The route names the WORKSPACE the feature chat owns, not the chat itself:
	// the frontend's /ide/:projectId/:repoId/:wsId route predates chat-scoping
	// and was not part of this refactor (spec §8 step 6 left it alone).
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
