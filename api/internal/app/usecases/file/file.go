package file

import (
	"context"
	"fmt"
	"time"

	"github.com/char2cs/crowbar/api/internal/domain"
	gitdomain "github.com/char2cs/crowbar/api/internal/domain/git"
	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

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

type FileStatusProvider interface {
	GitStatus(
		repoPath string,
	) (gitdomain.GitStatus, error)
}

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

type Usecase interface {
	Tree(
		ctx context.Context,
		wsID string,
		dirPath string,
		provider FileStatusProvider,
	) ([]domain.FileNode, error)

	ReadDocument(
		ctx context.Context,
		target domain.DocumentRef,
		transport domain.Transport,
	) (domain.Document, error)

	WriteDocument(
		ctx context.Context,
		target domain.DocumentRef,
		content string,
		encoding string,
		transport domain.Transport,
		now time.Time,
	) (domain.DocumentRef, error)

	HostDocument(
		ctx context.Context,
		path string,
		transport domain.Transport,
	) (domain.DocumentRef, error)

	CreateFile(
		ctx context.Context,
		wsID string,
		filePath string,
		now time.Time,
	) error

	CreateDir(
		ctx context.Context,
		wsID string,
		dirPath string,
		now time.Time,
	) error

	Copy(
		ctx context.Context,
		wsID string,
		sourcePath string,
		destPath string,
		now time.Time,
	) error

	Rename(
		ctx context.Context,
		wsID string,
		oldPath string,
		newPath string,
		now time.Time,
	) error

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
		return "", fmt.Errorf("file: write: workspace locked: %w", problem.ErrLocked)
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
