package stream

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

type GitStatuses interface {
	Status(ctx context.Context, wsID string) (gitdomain.GitStatus, error)
}

type GitSource struct {
	workspaces Workspaces
	git        GitStatuses
}

func NewGitSource(workspaces Workspaces, git GitStatuses) *GitSource {
	return &GitSource{workspaces: workspaces, git: git}
}

func (s *GitSource) Attach(ctx context.Context, wsID string, _ int64, emit func(stream.Frame)) error {
	if _, err := s.workspaces.Get(ctx, wsID); err != nil {
		return fmt.Errorf("stream: git snapshot: %w", err)
	}
	status, err := s.git.Status(ctx, wsID)
	if err != nil {
		slog.DebugContext(ctx, "stream: git snapshot", "workspace_id", wsID, "err", err)
		return nil
	}
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
