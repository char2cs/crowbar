package git

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/char2cs/crowbar/api/internal/core/binpath"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	gitexec "github.com/char2cs/crowbar/api/internal/engine/git/internal/exec"
)

// CurrentIdentity resolves the current human's identity for worktreePath.
//
// Resolution order:
//  1. `gh api user` — returns Login, DisplayName (name), and AvatarURL.
//  2. `git config user.name` — populates DisplayName only; Login and AvatarURL
//     are left empty.
//
// Neither step is allowed to fail the request: any subprocess error is silently
// swallowed and the next fallback is tried. The caller always receives a 200
// response with a best-effort identity (all fields may be empty).
func CurrentIdentity(
	ctx context.Context,
	worktreePath string,
) gitdomain.Identity {
	if id, ok := identityFromGH(ctx, worktreePath); ok {
		return id
	}
	return identityFromGitConfig(ctx, worktreePath)
}

// ghUserJSON is the subset of `gh api user` we care about.
type ghUserJSON struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

// identityFromGH tries `gh api user` in worktreePath. Returns (identity, true)
// on success, ("", false) on any failure so the caller can fall through.
func identityFromGH(
	ctx context.Context,
	worktreePath string,
) (gitdomain.Identity, bool) {
	// binpath.Resolve: the packaged .app daemon inherits launchd's minimal
	// PATH, which misses Homebrew's /opt/homebrew/bin where gh usually lives.
	//nolint:gosec // G204: fixed gh subcommand; binpath.Resolve locates the trusted gh binary.
	cmd := exec.CommandContext(ctx, binpath.Resolve("gh"), "api", "user")
	cmd.Dir = worktreePath

	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf

	if err := cmd.Run(); err != nil {
		return gitdomain.Identity{}, false
	}

	var u ghUserJSON
	if err := json.Unmarshal(out.Bytes(), &u); err != nil {
		return gitdomain.Identity{}, false
	}

	return gitdomain.Identity{
		Login:       strings.TrimSpace(u.Login),
		DisplayName: strings.TrimSpace(u.Name),
		AvatarURL:   strings.TrimSpace(u.AvatarURL),
	}, true
}

// identityFromGitConfig reads `git config user.name` from worktreePath. It
// never returns an error: on failure the DisplayName is simply empty.
func identityFromGitConfig(
	ctx context.Context,
	worktreePath string,
) gitdomain.Identity {
	r := gitexec.Git(ctx, worktreePath, "config", "user.name")
	if gitexec.RequireSuccess("git config user.name", r) != nil {
		return gitdomain.Identity{}
	}
	return gitdomain.Identity{
		DisplayName: strings.TrimSpace(r.Stdout),
	}
}
