// Package gitlab implements the GitProvider interface via the glab CLI.
package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

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

type glabProvider struct {
	execFn ExecFn
}

// New returns a glabProvider backed by the glab CLI.
func New() *glabProvider {
	return &glabProvider{execFn: withWaitDelay(exec.CommandContext)}
}

// NewWithExec returns a glabProvider that uses execFn for subprocess invocation.
// Intended for tests.
func NewWithExec(
	execFn ExecFn,
) *glabProvider {
	return &glabProvider{execFn: execFn}
}

// ProtectedBranches returns the list of protected branch names for the repo.
// It calls the GitLab API via glab and parses the JSON output.
func (g *glabProvider) ProtectedBranches(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	out, err := g.runGlab(
		ctx,
		repoPath,
		"api", "projects/:id/protected_branches",
		"--jq", ".[].name",
	)
	if err != nil {
		return nil, fmt.Errorf("gitlab: protected-branches: %w", err)
	}

	return parseLines(out), nil
}

// mrJSON is the structure returned by glab mr list --output json.
type mrJSON struct {
	IID          int    `json:"iid"`
	State        string `json:"state"`
	WebURL       string `json:"web_url"`
	Title        string `json:"title"`
	TargetBranch string `json:"target_branch"`
}

// PullRequestForBranch returns the most relevant MR for branch, or nil if none.
// Returns nil without calling the provider if branch has no upstream remote.
func (g *glabProvider) PullRequestForBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) (*providertypes.PRInfo, error) {
	hasUpstream, err := g.branchHasUpstream(ctx, repoPath, branch)
	if err != nil {
		return nil, fmt.Errorf("gitlab: mr-for-branch: upstream check: %w", err)
	}
	if !hasUpstream {
		return nil, nil
	}

	out, err := g.runGlab(
		ctx,
		repoPath,
		"mr", "list",
		"--source-branch", branch,
		"--state", "all",
		"--output", "json",
	)
	if err != nil {
		return nil, fmt.Errorf("gitlab: list-mrs: %w", err)
	}

	mrs, err := parseMRList([]byte(out))
	if err != nil {
		return nil, fmt.Errorf("gitlab: list-mrs: parse: %w", err)
	}

	best := selectBestMR(mrs)
	if best == nil {
		return nil, nil
	}

	return &providertypes.PRInfo{
		Number:       best.IID,
		Status:       mapState(best.State),
		URL:          best.WebURL,
		Title:        best.Title,
		TargetBranch: best.TargetBranch,
	}, nil
}

// parseMRList parses the JSON array returned by glab mr list.
func parseMRList(
	data []byte,
) ([]mrJSON, error) {
	var mrs []mrJSON
	if err := json.Unmarshal(data, &mrs); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return mrs, nil
}

// selectBestMR picks the most relevant MR from the list.
// Open MRs take priority; within the same state, higher IID wins.
func selectBestMR(
	mrs []mrJSON,
) *mrJSON {
	if len(mrs) == 0 {
		return nil
	}

	var best *mrJSON
	for i := range mrs {
		mr := &mrs[i]
		if best == nil {
			best = mr
			continue
		}
		best = betterMR(best, mr)
	}
	return best
}

// betterMR returns the more relevant of two MRs.
func betterMR(
	a *mrJSON,
	b *mrJSON,
) *mrJSON {
	aOpen := strings.EqualFold(a.State, "opened")
	bOpen := strings.EqualFold(b.State, "opened")

	if aOpen && !bOpen {
		return a
	}
	if bOpen && !aOpen {
		return b
	}
	if b.IID > a.IID {
		return b
	}
	return a
}

// mapState maps glab CLI state strings to canonical status values.
func mapState(
	state string,
) string {
	switch strings.ToLower(state) {
	case "opened":
		return "open"
	case "merged":
		return "merged"
	default:
		return "closed"
	}
}

// branchHasUpstream checks whether branch exists on the remote.
func (g *glabProvider) branchHasUpstream(
	ctx context.Context,
	repoPath string,
	branch string,
) (bool, error) {
	ref := "refs/heads/" + branch
	cmd := g.execFn(ctx, "git", "ls-remote", "origin", ref)
	cmd.Dir = repoPath

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return false, fmt.Errorf("ls-remote: stderr=%q: %w", strings.TrimSpace(errBuf.String()), err)
	}
	return strings.TrimSpace(out.String()) != "", nil
}

// runGlab executes a glab command in repoPath and returns stdout.
func (g *glabProvider) runGlab(
	ctx context.Context,
	repoPath string,
	args ...string,
) (string, error) {
	cmd := g.execFn(ctx, "glab", args...)
	cmd.Dir = repoPath

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("glab %s: stderr=%q: %w", args[0], strings.TrimSpace(errBuf.String()), err)
	}
	return out.String(), nil
}

// OwnerAvatarURL returns the GitLab namespace avatar URL for the repo.
// Returns ("", nil) on any soft failure so callers can fall back gracefully.
func (g *glabProvider) OwnerAvatarURL(
	ctx context.Context,
	repoPath string,
) (string, error) {
	out, err := g.runGlab(ctx, repoPath, "api", "projects/:id", "--jq", ".namespace.avatar_url")
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
