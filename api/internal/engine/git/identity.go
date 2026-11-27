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

func CurrentIdentity(
	ctx context.Context,
	worktreePath string,
) gitdomain.Identity {
	if id, ok := identityFromGH(ctx, worktreePath); ok {
		return id
	}
	return identityFromGitConfig(ctx, worktreePath)
}

type ghUserJSON struct {
	Login     string `json:"login"`
	Name      string `json:"name"`
	AvatarURL string `json:"avatar_url"`
}

func identityFromGH(
	ctx context.Context,
	worktreePath string,
) (gitdomain.Identity, bool) {
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
