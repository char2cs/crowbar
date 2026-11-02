package stream

import (
	"context"
	"fmt"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/domain/stream"
)

// OperationSnapshots is where the projects scope reads every retained operation.
type OperationSnapshots interface {
	Snapshot(ctx context.Context) ([]domain.Operation, error)
}

// Sidebar is where the projects scope reads every project, repo, workspace,
// chat and folder, each as the list routes serve it.
type Sidebar interface {
	Entities(ctx context.Context) (Snapshot, error)
}

// ProjectsSource serves the projects scope.
type ProjectsSource struct {
	ops     OperationSnapshots
	sidebar Sidebar
	encode  Encoder
}

// NewProjectsSource builds the projects scope's source.
func NewProjectsSource(ops OperationSnapshots, sidebar Sidebar, encode Encoder) *ProjectsSource {
	return &ProjectsSource{ops: ops, sidebar: sidebar, encode: encode}
}

// Attach emits every retained operation; projects takes no key and no since.
func (s *ProjectsSource) Attach(ctx context.Context, _ string, _ int64, emit func(stream.Frame)) error {
	ops, err := s.ops.Snapshot(ctx)
	if err != nil {
		return fmt.Errorf("stream: projects snapshot: %w", err)
	}
	for _, op := range ops {
		data, err := s.encode(stream.KindOperation, op)
		if err != nil {
			return fmt.Errorf("stream: projects snapshot: operation %s: %w", op.ID, err)
		}
		emit(stream.Frame{T: stream.FrameEntity, Kind: string(stream.KindOperation), ID: op.ID, Data: data})
	}
	return nil
}

// Read is the sidebar's entities: slow to read, and its reads may write.
func (s *ProjectsSource) Read(ctx context.Context, _ string) (Snapshot, error) {
	snap, err := s.sidebar.Entities(ctx)
	if err != nil {
		return Snapshot{}, fmt.Errorf("stream: projects snapshot: %w", err)
	}
	return snap, nil
}
