package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// GitStatuses is where the git scope reads a workspace's current status.
type GitStatuses interface {
	Status(ctx context.Context, wsID string) (gitdomain.GitStatus, error)
}

// GitSource serves the git scope: the workspace's current status as one
// signal, so a change between the client's own read and its hold is not lost.
type GitSource struct {
	workspaces Workspaces
	git        GitStatuses
}

// NewGitSource builds the git scope's source.
func NewGitSource(workspaces Workspaces, git GitStatuses) *GitSource {
	return &GitSource{workspaces: workspaces, git: git}
}

// Attach emits wsID's status; an unknown workspace is not found, and a status
// that cannot be read is left to the watcher's next signal.
func (s *GitSource) Attach(ctx context.Context, wsID string, _ int64, emit func(stream.Frame)) error {
	if _, err := s.workspaces.Get(ctx, wsID); err != nil {
		return fmt.Errorf("stream: git snapshot: %w", err)
	}
	status, err := s.git.Status(ctx, wsID)
	if err != nil {
		slog.DebugContext(ctx, "stream: git snapshot", "workspace_id", wsID, "err", err)
		return nil
	}
	// A clean tree carries a nil Files slice; the client reads files: [], never null.
	if status.Files == nil {
		status.Files = []gitdomain.GitFile{}
	}
	data, err := json.Marshal(status)
	if err != nil {
		return fmt.Errorf("stream: git snapshot: %w", err)
	}
	emit(stream.Frame{T: stream.FrameSignal, Kind: stream.SignalGitStatus, Data: data})
	return nil
}
