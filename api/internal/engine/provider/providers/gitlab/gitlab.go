package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	providertypes "github.com/char2cs/crowbar/api/internal/engine/provider/types"
)

type ExecFn func(ctx context.Context, name string, args ...string) *exec.Cmd

type glabProvider struct {
	execFn ExecFn
}

func New(
	execFn ExecFn,
) *glabProvider {
	return &glabProvider{execFn: execFn}
}

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

type mrJSON struct {
	IID          int    `json:"iid"`
	State        string `json:"state"`
	WebURL       string `json:"web_url"`
	Title        string `json:"title"`
	SourceBranch string `json:"source_branch"`
	TargetBranch string `json:"target_branch"`
	SHA          string `json:"sha"`
}

func (g *glabProvider) PullRequestForBranch(
	ctx context.Context,
	repoPath string,
	branch string,
) (*providertypes.PRInfo, error) {
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
	if !g.ownsMR(ctx, repoPath, branch, best) {
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

func (g *glabProvider) OpenPullRequests(
	ctx context.Context,
	repoPath string,
) ([]providertypes.PRLink, error) {
	out, err := g.runGlab(
		ctx,
		repoPath,
		"mr", "list",
		"--state", "opened",
		"--output", "json",
	)
	if err != nil {
		return nil, fmt.Errorf("gitlab: open-mrs: %w", err)
	}
	mrs, err := parseMRList([]byte(out))
	if err != nil {
		return nil, fmt.Errorf("gitlab: open-mrs: parse: %w", err)
	}
	links := make([]providertypes.PRLink, 0, len(mrs))
	for i := range mrs {
		m := &mrs[i]
		links = append(links, providertypes.PRLink{
			Head:   m.SourceBranch,
			Base:   m.TargetBranch,
			Number: m.IID,
			Status: mapState(m.State),
			URL:    m.WebURL,
			Title:  m.Title,
		})
	}
	return links, nil
}

func (g *glabProvider) ownsMR(
	ctx context.Context,
	repoPath string,
	branch string,
	mr *mrJSON,
) bool {
	if strings.EqualFold(mr.State, "opened") {
		return true
	}
	if mr.SHA == "" {
		return true
	}
	return g.branchContains(ctx, repoPath, branch, mr.SHA)
}

func (g *glabProvider) branchContains(
	ctx context.Context,
	repoPath string,
	branch string,
	sha string,
) bool {
	cmd := g.execFn(ctx, binpath.Git(), "merge-base", "--is-ancestor", sha, branch)
	cmd.Dir = repoPath
	return cmd.Run() == nil
}

func parseMRList(
	data []byte,
) ([]mrJSON, error) {
	var mrs []mrJSON
	if err := json.Unmarshal(data, &mrs); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return mrs, nil
}

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

func (g *glabProvider) runGlab(
	ctx context.Context,
	repoPath string,
	args ...string,
) (string, error) {
	cmd := g.execFn(ctx, binpath.Resolve("glab"), args...)
	cmd.Dir = repoPath

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return "", &providertypes.CLIError{Tool: "glab", Verb: args[0], Stderr: strings.TrimSpace(errBuf.String()), Err: err}
	}
	return out.String(), nil
}

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
