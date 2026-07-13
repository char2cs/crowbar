// Package github implements the GitProvider interface via the gh CLI.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

// ExecFn matches exec.CommandContext so callers can inject a test stub.
type ExecFn func(ctx context.Context, name string, args ...string) *exec.Cmd

// waitDelay bounds how long Cmd.Wait blocks for I/O to drain after the context
// is cancelled (e.g. on poll timeout). Without it a subprocess whose stdout pipe
// is inherited by a still-running grandchild keeps the goroutine and the process
// entry alive indefinitely, leaking on every hung poll.
const waitDelay = 10 * time.Second

// withWaitDelay wraps an ExecFn so every constructed Cmd carries a WaitDelay,
// guaranteeing a killed subprocess releases even when its pipes are held open.
func withWaitDelay(
	execFn ExecFn,
) ExecFn {
	return func(
		ctx context.Context,
		name string,
		args ...string,
	) *exec.Cmd {
		cmd := execFn(ctx, name, args...)
		cmd.WaitDelay = waitDelay
		return cmd
	}
}

type ghProvider struct {
	execFn ExecFn
}

// New returns a ghProvider backed by the gh CLI.
func New() *ghProvider {
	return &ghProvider{execFn: withWaitDelay(exec.CommandContext)}
}

// NewWithExec returns a ghProvider that uses execFn for subprocess invocation.
// Intended for tests.
func NewWithExec(
	execFn ExecFn,
) *ghProvider {
	return &ghProvider{execFn: execFn}
}

// ProtectedBranches returns the list of protected branch names for the repo.
// It calls the GitHub API via gh and parses the newline-delimited output.
func (g *ghProvider) ProtectedBranches(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	s, err := slug(ctx, repoPath, g.execFn)
	if err != nil {
		return nil, fmt.Errorf("github: protected-branches: %w", err)
	}

	path := fmt.Sprintf("repos/%s/branches", s)
	out, err := g.runGH(
		ctx,
		repoPath,
		"api", path,
		"--paginate",
		"--jq", ".[] | select(.protected) | .name",
	)
	if err != nil {
		return nil, fmt.Errorf("github: protected-branches: %w", err)
	}

	return parseLines(out), nil
}

// prJSON is the structure returned by gh pr list --json.
type prJSON struct {
	Number      int    `json:"number"`
	State       string `json:"state"`
	URL         string `json:"url"`
	Title       string `json:"title"`
	BaseRefName string `json:"baseRefName"`
	HeadRefOid  string `json:"headRefOid"`
}

// PullRequestForBranch returns the most relevant PR for branch, or nil if none.
//
// The lookup deliberately does NOT check whether branch still exists on the
// remote first: GitHub deletes the head ref when a PR merges, so that check goes
// false at exactly the moment the answer changes, and the merge would never be
// observed. gh reports a merged PR for a deleted head ref just fine.
func (g *ghProvider) PullRequestForBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) (*providertypes.PRInfo, error) {
	out, err := g.runGH(
		ctx,
		repoPath,
		"pr", "list",
		"--head", branch,
		"--json", "number,state,url,title,baseRefName,headRefOid",
		"--state", "all",
	)
	if err != nil {
		return nil, fmt.Errorf("github: list-prs: %w", err)
	}

	prs, err := parsePRList([]byte(out))
	if err != nil {
		return nil, fmt.Errorf("github: list-prs: parse: %w", err)
	}

	best := selectBestPR(prs)
	if best == nil {
		return nil, nil
	}
	if !g.ownsPR(ctx, repoPath, branch, best) {
		return nil, nil
	}

	return &providertypes.PRInfo{
		Number:       best.Number,
		Status:       mapState(best.State),
		URL:          best.URL,
		Title:        best.Title,
		TargetBranch: best.BaseRefName,
	}, nil
}

// ownsPR reports whether pr is really this branch's PR.
//
// gh matches PRs by head branch NAME alone, and a merged PR outlives the ref it
// was opened from. So reusing a merged branch's name for fresh work matches that
// stale PR — the workspace would show pr-merged, linked to someone else's history,
// and pr-merged is terminal: the sweep drops the workspace and never revisits it.
//
// An open PR still owns its head ref, so a name match is authoritative. A
// merged/closed PR belongs to this branch only if the branch still contains the
// commit the PR was built from.
func (g *ghProvider) ownsPR(
	ctx context.Context,
	repoPath string,
	branch string,
	pr *prJSON,
) bool {
	if strings.EqualFold(pr.State, "open") {
		return true
	}
	if pr.HeadRefOid == "" {
		return true
	}
	return g.branchContains(ctx, repoPath, branch, pr.HeadRefOid)
}

// branchContains reports whether sha is an ancestor of (or equal to) branch.
//
// A non-zero exit means "not an ancestor" and an unknown object means git cannot
// answer; both resolve to false. That direction is deliberate: a false negative
// leaves the status stale, which is merely the old behaviour, while a false
// positive brands the workspace with a foreign PR and retires it from the sweep.
func (g *ghProvider) branchContains(
	ctx context.Context,
	repoPath string,
	branch string,
	sha string,
) bool {
	cmd := g.execFn(ctx, "git", "merge-base", "--is-ancestor", sha, branch)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

// parsePRList parses the JSON array returned by gh pr list.
func parsePRList(
	data []byte,
) ([]prJSON, error) {
	var prs []prJSON
	if err := json.Unmarshal(data, &prs); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return prs, nil
}

// selectBestPR picks the most relevant PR from the list.
// Open PRs take priority; within the same state, higher number wins.
func selectBestPR(
	prs []prJSON,
) *prJSON {
	if len(prs) == 0 {
		return nil
	}

	var best *prJSON
	for i := range prs {
		pr := &prs[i]
		if best == nil {
			best = pr
			continue
		}
		best = betterPR(best, pr)
	}
	return best
}

// betterPR returns the more relevant of two PRs.
func betterPR(
	a *prJSON,
	b *prJSON,
) *prJSON {
	aOpen := strings.EqualFold(a.State, "open")
	bOpen := strings.EqualFold(b.State, "open")

	if aOpen && !bOpen {
		return a
	}
	if bOpen && !aOpen {
		return b
	}
	if b.Number > a.Number {
		return b
	}
	return a
}

// mapState maps gh CLI state strings to canonical status values.
func mapState(
	state string,
) string {
	switch strings.ToUpper(state) {
	case "OPEN":
		return "open"
	case "MERGED":
		return "merged"
	default:
		return "closed"
	}
}

// runGH executes a gh command in repoPath and returns stdout.
func (g *ghProvider) runGH(
	ctx context.Context,
	repoPath string,
	args ...string,
) (string, error) {
	// binpath.Resolve: the packaged .app daemon inherits launchd's minimal
	// PATH, which misses Homebrew's /opt/homebrew/bin where gh usually lives.
	cmd := g.execFn(ctx, binpath.Resolve("gh"), args...)
	cmd.Dir = repoPath

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: stderr=%q: %w", args[0], strings.TrimSpace(errBuf.String()), err)
	}
	return out.String(), nil
}

// OwnerAvatarURL returns the GitHub owner's avatar URL for the repo.
// Returns ("", nil) on any soft failure so callers can fall back gracefully.
func (g *ghProvider) OwnerAvatarURL(
	ctx context.Context,
	repoPath string,
) (string, error) {
	s, err := slug(ctx, repoPath, g.execFn)
	if err != nil {
		return "", nil
	}
	path := fmt.Sprintf("repos/%s", s)
	out, err := g.runGH(ctx, repoPath, "api", path, "--jq", ".owner.avatar_url")
	if err != nil {
		return "", nil
	}
	return strings.TrimSpace(out), nil
}

// parseLines splits newline-delimited output into non-empty strings.
func parseLines(
	s string,
) []string {
	raw := strings.Split(strings.TrimSpace(s), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
