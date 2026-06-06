package file

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceSyncer is the workspace surface the file and git usecases use to
// resolve a worktree path and trigger a working-tree resync after a mutation.
type WorkspaceSyncer interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	SyncWorkingTreeState(
		ctx context.Context,
		id string,
		now time.Time,
	) (domain.Workspace, error)
}

// FileStatusProvider resolves git status for a repo path so the file tree can be
// decorated. It is structurally compatible with the fs engine's tree provider.
type FileStatusProvider interface {
	GitStatus(
		repoPath string,
	) (gitdomain.GitStatus, error)
}

// FsEngine is the filesystem-engine surface the file usecase passes through to.
type FsEngine interface {
	Tree(
		repoPath string,
		dirPath string,
		provider FileStatusProvider,
	) ([]domain.FileNode, error)
	ReadContent(
		repoPath string,
		filePath string,
	) (domain.FileContent, error)
	WriteContent(
		repoPath string,
		filePath string,
		content string,
	) error
	CreateFile(
		repoPath string,
		filePath string,
	) error
	CreateDir(
		repoPath string,
		dirPath string,
	) error
	Rename(
		repoPath string,
		oldPath string,
		newPath string,
	) error
	Delete(
		repoPath string,
		filePath string,
	) error
}

// Usecase is a thin pass-through to the filesystem engine. Reads resolve the
// worktree path; mutations additionally trigger a working-tree resync.
type Usecase interface {
	// Tree returns one level of the file tree for a workspace.
	Tree(
		ctx context.Context,
		wsID string,
		dirPath string,
		provider FileStatusProvider,
	) ([]domain.FileNode, error)

	// ReadContent reads a file in a workspace.
	ReadContent(
		ctx context.Context,
		wsID string,
		filePath string,
	) (domain.FileContent, error)

	// WriteContent writes a file and resyncs the working tree.
	WriteContent(
		ctx context.Context,
		wsID string,
		filePath string,
		content string,
		now time.Time,
	) error

	// CreateFile creates a file and resyncs the working tree.
	CreateFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error

	// CreateDir creates a directory and resyncs the working tree.
	CreateDir(
		ctx context.Context,
		wsID string,
		dirPath string,
		now time.Time,
	) error

	// Rename renames a path and resyncs the working tree.
	Rename(
		ctx context.Context,
		wsID string,
		oldPath string,
		newPath string,
		now time.Time,
	) error

	// Delete removes a path and resyncs the working tree.
	Delete(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error
}

type fileUsecase struct {
	fs     FsEngine
	syncer WorkspaceSyncer
}

// New builds a Usecase from the fs engine and the workspace syncer.
func New(
	fs FsEngine,
	syncer WorkspaceSyncer,
) Usecase {
	return &fileUsecase{
		fs:     fs,
		syncer: syncer,
	}
}

// Tree returns one level of the file tree for a workspace.
func (u *fileUsecase) Tree(
	ctx context.Context,
	wsID string,
	dirPath string,
	provider FileStatusProvider,
) ([]domain.FileNode, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return nil, err
	}
	nodes, err := u.fs.Tree(repoPath, dirPath, provider)
	if err != nil {
		return nil, fmt.Errorf("file: tree: %w", err)
	}
	return nodes, nil
}

// ReadContent reads a file in a workspace.
func (u *fileUsecase) ReadContent(
	ctx context.Context,
	wsID string,
	filePath string,
) (domain.FileContent, error) {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return domain.FileContent{}, err
	}
	content, err := u.fs.ReadContent(repoPath, filePath)
	if err != nil {
		return domain.FileContent{}, fmt.Errorf("file: read content: %w", err)
	}
	return content, nil
}

// WriteContent writes a file and resyncs the working tree.
func (u *fileUsecase) WriteContent(
	ctx context.Context,
	wsID string,
	filePath string,
	content string,
	now time.Time,
) error {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return err
	}
	if err := u.fs.WriteContent(repoPath, filePath, content); err != nil {
		return fmt.Errorf("file: write content: %w", err)
	}
	return u.resync(ctx, wsID, now)
}

// CreateFile creates a file and resyncs the working tree.
func (u *fileUsecase) CreateFile(
	ctx context.Context,
	wsID string,
	filePath string,
	now time.Time,
) error {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return err
	}
	if err := u.fs.CreateFile(repoPath, filePath); err != nil {
		return fmt.Errorf("file: create file: %w", err)
	}
	return u.resync(ctx, wsID, now)
}

// CreateDir creates a directory and resyncs the working tree.
func (u *fileUsecase) CreateDir(
	ctx context.Context,
	wsID string,
	dirPath string,
	now time.Time,
) error {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return err
	}
	if err := u.fs.CreateDir(repoPath, dirPath); err != nil {
		return fmt.Errorf("file: create dir: %w", err)
	}
	return u.resync(ctx, wsID, now)
}

// Rename renames a path and resyncs the working tree.
func (u *fileUsecase) Rename(
	ctx context.Context,
	wsID string,
	oldPath string,
	newPath string,
	now time.Time,
) error {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return err
	}
	if err := u.fs.Rename(repoPath, oldPath, newPath); err != nil {
		return fmt.Errorf("file: rename: %w", err)
	}
	return u.resync(ctx, wsID, now)
}

// Delete removes a path and resyncs the working tree.
func (u *fileUsecase) Delete(
	ctx context.Context,
	wsID string,
	filePath string,
	now time.Time,
) error {
	repoPath, err := u.repoPath(ctx, wsID)
	if err != nil {
		return err
	}
	if err := u.fs.Delete(repoPath, filePath); err != nil {
		return fmt.Errorf("file: delete: %w", err)
	}
	return u.resync(ctx, wsID, now)
}

func (u *fileUsecase) repoPath(
	ctx context.Context,
	wsID string,
) (string, error) {
	ws, err := u.syncer.Get(ctx, wsID)
	if err != nil {
		return "", fmt.Errorf("file: load workspace: %w", err)
	}
	return ws.WorktreePath, nil
}

func (u *fileUsecase) resync(
	ctx context.Context,
	wsID string,
	now time.Time,
) error {
	if _, err := u.syncer.SyncWorkingTreeState(ctx, wsID, now); err != nil {
		return fmt.Errorf("file: resync: %w", err)
	}
	return nil
}
