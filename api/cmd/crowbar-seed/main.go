// Command crowbar-seed fills a running DEV daemon with a throwaway git repo, a project, a feature
// workspace with a real diff and two review threads on it. It is its own binary so dev tooling
// never ships; run it via `make seed` (dev CROWBAR_HOME). Every step is idempotent.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/home"
)

func main() {
	host := flag.String("host", "unix://",
		"daemon address: unix:// for the desktop sidecar, tcp://127.0.0.1:3737 for `make dev-api` (plaintext and unauthenticated: dev only)")
	flag.Parse()

	if err := run(context.Background(), *host, os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context,
	host string,
	out io.Writer,
) error {
	root, err := seedRoot()
	if err != nil {
		return err
	}
	repoPath := filepath.Join(root, seedRepoName)
	rep := &report{out: out}

	fixture, fixtureCreated, err := ensureFixture(repoPath)
	if err != nil {
		return err
	}
	rep.note(fixtureCreated, "git fixture at "+repoPath)

	wire, err := newTransport(host)
	if err != nil {
		return fmt.Errorf("seed: connect to %s: %w", host, err)
	}
	d := &daemon{wire: wire}

	project, projectCreated, err := ensureProject(ctx, d, root)
	if err != nil {
		return err
	}
	rep.project = project
	rep.note(projectCreated, "project "+seedProjectName)

	repo, repoCreated, err := ensureRepo(ctx, d, project.ID, repoPath)
	if err != nil {
		return err
	}
	rep.repo = repo
	rep.note(repoCreated, "repo "+seedRepoName+" on "+repo.DefaultBranch)

	return finish(ctx, d, fixture, rep)
}

// finish covers everything downstream of the imported repo: the base chat
// import provisions, the feature chat forked off it, the commit that gives the
// review surface something to render, and the threads on that commit.
func finish(
	ctx context.Context,
	d *daemon,
	fixture *gitRepo,
	rep *report,
) error {
	base, err := waitForBaseChat(ctx, d, rep.repo)
	if err != nil {
		return err
	}

	chat, chatCreated, err := ensureFeatureChat(ctx, d, rep.repo, base.ID)
	if err != nil {
		return err
	}
	rep.workspace = chat
	rep.note(chatCreated, "workspace "+seedFeatureBranch)

	worktree, repaired, err := ensureWorktree(fixture, chat.Worktree.LocalPath, chat.Worktree.Branch)
	if err != nil {
		return err
	}
	if repaired {
		rep.noteRepair("worktree for " + seedFeatureBranch + " at " + chat.Worktree.LocalPath)
	}

	diffCreated, err := ensureBranchDiff(worktree)
	if err != nil {
		return err
	}
	rep.note(diffCreated, "review commit on "+chat.Worktree.Branch)

	sc := scope{projectID: rep.repo.ProjectID, repoID: rep.repo.ID, workspaceID: chat.WorkspaceID}
	threads, err := ensureThreads(ctx, d, sc, resolveAuthor(ctx, d, chat.ID))
	if err != nil {
		return err
	}
	rep.noteThreads(threads)

	return rep.write()
}

// seedRoot puts the throwaway repo under CROWBAR_HOME. A dev home is required,
// not defaulted: falling back to the user's real home would seed production when
// the binary runs outside `make seed`.
func seedRoot() (string, error) {
	h, err := home.Resolve()
	if err != nil {
		return "", fmt.Errorf("seed: resolve home: %w", err)
	}
	if h.IsDefault() {
		return "", errors.New(
			"seed: CROWBAR_HOME does not name a dev home — run this through `make seed` " +
				"(or export CROWBAR_HOME yourself); crowbar-seed refuses to guess " +
				"and never writes to the user's own Crowbar home",
		)
	}
	return filepath.Join(h.Root(), "seed"), nil
}
