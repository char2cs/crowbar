package stream

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// Workspaces is where a workspace scope's source confirms its workspace exists.
type Workspaces interface {
	Get(ctx context.Context, id string) (domain.Workspace, error)
}

// ReviewThreads is where the ws scope reads a workspace's review threads.
type ReviewThreads interface {
	ListByWorkspace(ctx context.Context, wsID string) ([]domain.ReviewThread, error)
}

// ReviewThread is a review thread with the project and repo its wire form names.
type ReviewThread struct {
	Thread    domain.ReviewThread
	ProjectID string
	RepoID    string
}

// WsSource serves the ws scope: every review thread of the workspace.
type WsSource struct {
	workspaces Workspaces
	threads    ReviewThreads
	encode     Encoder
}

// NewWsSource builds the ws scope's source.
func NewWsSource(workspaces Workspaces, threads ReviewThreads, encode Encoder) *WsSource {
	return &WsSource{workspaces: workspaces, threads: threads, encode: encode}
}

// Attach emits wsID's review threads; an unknown workspace is not found.
func (s *WsSource) Attach(ctx context.Context, wsID string, _ int64, emit func(stream.Frame)) error {
	ws, err := s.workspaces.Get(ctx, wsID)
	if err != nil {
		return fmt.Errorf("stream: ws snapshot: %w", err)
	}
	threads, err := s.threads.ListByWorkspace(ctx, wsID)
	if err != nil {
		return fmt.Errorf("stream: ws snapshot: threads of %s: %w", wsID, err)
	}
	for _, th := range threads {
		data, err := s.encode(stream.KindReviewThread, ReviewThread{Thread: th, ProjectID: ws.ProjectID, RepoID: ws.RepoID})
		if err != nil {
			return fmt.Errorf("stream: ws snapshot: thread %s: %w", th.ID, err)
		}
		emit(stream.Frame{T: stream.FrameEntity, Kind: string(stream.KindReviewThread), ID: th.ID, Data: data})
	}
	return nil
}
