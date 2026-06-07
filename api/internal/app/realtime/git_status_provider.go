package realtime

import (
	"context"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	enginegit "github.com/char2cs/crowbar/api/internal/engine/git"
)

// gitStatusProvider adapts the git engine to the watch.GitStatusProvider
// surface the file watcher consumes (05 §5). It exists to invert the dependency
// so the watch package never imports the git engine directly.
type gitStatusProvider struct {
	engine enginegit.Engine
}

func (p *gitStatusProvider) ComputeStatus(
	ctx context.Context,
	repoPath string,
) (gitdomain.GitStatus, error) {
	return p.engine.Status(ctx, repoPath)
}

func (p *gitStatusProvider) ComputeWorkingTreeSummary(
	ctx context.Context,
	repoPath string,
	forkPointSha string,
) (int, int, bool, bool, error) {
	return p.engine.WorkingTreeSummary(ctx, repoPath, forkPointSha)
}
