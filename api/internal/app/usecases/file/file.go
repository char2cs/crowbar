package file

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/app/apperr"
	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
)

// WorkspaceSyncer is the workspace surface the file usecase uses to resolve a
// worktree path, find the worktree holding an absolute path, and trigger a
// working-tree resync after a mutation.
type WorkspaceSyncer interface {
	Get(
		ctx context.Context,
		id string,
	) (domain.Workspace, error)
	List(
		ctx context.Context,
	) ([]domain.Workspace, error)
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
		encoding string,
	) error
	ReadHostContent(
		absPath string,
	) (domain.FileContent, error)
	WriteHostContent(
		absPath string,
		content string,
		encoding string,
	) error
	CreateFile(
		repoPath string,
		filePath string,
	) error
	CreateDir(
		repoPath string,
		dirPath string,
	) error
	Copy(
		repoPath string,
		sourcePath string,
		destPath string,
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

	// ReadDocument reads the document target names: a workspace-relative path,
	// or an absolute path resolved to the worktree holding it or to the host.
	ReadDocument(
		ctx context.Context,
		target domain.DocumentRef,
		transport domain.Transport,
	) (domain.Document, error)

	// WriteDocument writes the document target names and returns the ref it
	// resolved to. encoding is "base64" for a byte-faithful binary payload (e.g.
	// a file upload) or "" / "utf8" for raw UTF-8 text.
	WriteDocument(
		ctx context.Context,
		target domain.DocumentRef,
		content string,
		encoding string,
		transport domain.Transport,
		now time.Time,
	) (domain.DocumentRef, error)

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

	// Copy duplicates a file or directory byte-faithfully and resyncs the
	// working tree. Directories copy recursively; the destination must not
	// already exist.
	Copy(
		ctx context.Context,
		wsID string,
		sourcePath string,
		destPath string,
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
	fs          FsEngine
	syncer      WorkspaceSyncer
	crowbarHome func() (string, error)
}

// New builds a Usecase from the fs engine, the workspace syncer and the
// daemon's home, whose state directory is never served as a document.
func New(
	fs FsEngine,
	syncer WorkspaceSyncer,
	crowbarHome func() (string, error),
) Usecase {
	return &fileUsecase{
		fs:          fs,
		syncer:      syncer,
		crowbarHome: crowbarHome,
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

// CreateFile creates a file and resyncs the working tree.
func (u *fileUsecase) CreateFile(
	ctx context.Context,
	wsID string,
	filePath string,
	now time.Time,
) error {
	repoPath, err := u.writePath(ctx, wsID)
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
	repoPath, err := u.writePath(ctx, wsID)
	if err != nil {
		return err
	}
	if err := u.fs.CreateDir(repoPath, dirPath); err != nil {
		return fmt.Errorf("file: create dir: %w", err)
	}
	return u.resync(ctx, wsID, now)
}

// Copy duplicates a file or directory byte-faithfully and resyncs the working
// tree. Locked (protected-branch) workspaces reject the write with
// apperr.ErrLocked, same as every other mutation.
func (u *fileUsecase) Copy(
	ctx context.Context,
	wsID string,
	sourcePath string,
	destPath string,
	now time.Time,
) error {
	repoPath, err := u.writePath(ctx, wsID)
	if err != nil {
		return err
	}
	if err := u.fs.Copy(repoPath, sourcePath, destPath); err != nil {
		return fmt.Errorf("file: copy: %w", err)
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
	repoPath, err := u.writePath(ctx, wsID)
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
	repoPath, err := u.writePath(ctx, wsID)
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

func (u *fileUsecase) writePath(
	ctx context.Context,
	wsID string,
) (string, error) {
	ws, err := u.syncer.Get(ctx, wsID)
	if err != nil {
		return "", fmt.Errorf("file: load workspace: %w", err)
	}
	if ws.Status == domain.WorkspaceStatusLocked {
		return "", fmt.Errorf("file: write: workspace locked: %w", apperr.ErrLocked)
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
