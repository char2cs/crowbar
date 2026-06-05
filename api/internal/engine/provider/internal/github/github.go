// Package github implements the GitProvider interface via the gh CLI.
package github

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/char2cs/crowbar/api/internal/engine/provider/internal/remote"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/internal/types"
)

// ExecFn is an alias for remote.ExecCommandFn so callers can inject a test stub.
type ExecFn = remote.ExecCommandFn

// GitProvider is the read-only interface this package exposes.
type GitProvider interface {
	ProtectedBranches(
		ctx context.Context,
		repoPath string,
	) ([]string, error)

	PullRequestForBranch(
		ctx context.Context,
		repoPath string,
		branch string,
	) (*providertypes.PRInfo, error)
}

type ghProvider struct {
	execFn ExecFn
}

// New returns a GitProvider backed by the gh CLI.
func New() GitProvider {
	return &ghProvider{execFn: exec.CommandContext}
}

// NewWithExec returns a GitProvider that uses execFn for subprocess invocation.
// Intended for tests.
func NewWithExec(
	execFn ExecFn,
) GitProvider {
	return &ghProvider{execFn: execFn}
}

// ProtectedBranches returns the list of protected branch names for the repo.
// It calls the GitHub API via gh and parses the newline-delimited output.
func (g *ghProvider) ProtectedBranches(
	ctx context.Context,
	repoPath string,
) ([]string, error) {
	slug, err := remote.Slug(ctx, repoPath, g.execFn)
	if err != nil {
		return nil, fmt.Errorf("github: protected-branches: %w", err)
	}

	path := fmt.Sprintf("repos/%s/branches", slug)
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
}

// PullRequestForBranch returns the most relevant PR for branch, or nil if none.
// Returns nil without calling the provider if branch has no upstream remote.
func (g *ghProvider) PullRequestForBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) (*providertypes.PRInfo, error) {
	hasUpstream, err := g.branchHasUpstream(ctx, repoPath, branch)
	if err != nil {
		return nil, fmt.Errorf("github: pr-for-branch: upstream check: %w", err)
	}
	if !hasUpstream {
		return nil, nil
	}

	out, err := g.runGH(
		ctx,
		repoPath,
		"pr", "list",
		"--head", branch,
		"--json", "number,state,url,title,baseRefName",
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

	return &providertypes.PRInfo{
		Number:       best.Number,
		Status:       mapState(best.State),
		URL:          best.URL,
		Title:        best.Title,
		TargetBranch: best.BaseRefName,
	}, nil
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

// branchHasUpstream checks whether branch exists on the remote.
func (g *ghProvider) branchHasUpstream(
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

// runGH executes a gh command in repoPath and returns stdout.
func (g *ghProvider) runGH(
	ctx context.Context,
	repoPath string,
	args ...string,
) (string, error) {
	cmd := g.execFn(ctx, "gh", args...)
	cmd.Dir = repoPath

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("gh %s: stderr=%q: %w", args[0], strings.TrimSpace(errBuf.String()), err)
	}
	return out.String(), nil
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
