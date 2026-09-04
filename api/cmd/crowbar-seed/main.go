// Command crowbar-seed fills a running Crowbar DEV daemon with usable state:
// a generated throwaway git repo, a project and repo importing it, a feature
// workspace with a real diff on it, and two review threads on that diff. It
// exists so starting work on a new workspace does not begin with ten minutes of
// clicking through import dialogs.
//
// It is deliberately its own binary and not a `crowbar` subcommand: this is dev
// tooling and must not ship. Run it through `make seed`, which exports the same
// CROWBAR_HOME the dev targets use.
//
// Everything it does is idempotent. A second run reuses the project, repo,
// workspace, commit and threads the first one created, and says so.
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/char2cs/crowbar/api/internal/core/metadata"
)

func main() {
	host := flag.String("host", "unix://",
		"daemon address: unix:// for the desktop sidecar, tcp://127.0.0.1:3737 for `make dev-api`")
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

// seedRoot puts the throwaway repo under the crowbar home. It is disposable
// state that already lives outside the source tree, and under a dev CROWBAR_HOME
// it is thrown away with the rest of the dev instance.
func seedRoot() (string, error) {
	home := os.Getenv(metadata.HomeEnvVar)
	if home == "" {
		userHome, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("seed: resolve home: %w", err)
		}
		home = filepath.Join(userHome, ".crowbar")
	}
	return filepath.Join(home, "seed"), nil
}
