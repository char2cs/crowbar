package fs

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/content"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/mutate"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/tree"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"
)

type Engine interface {
	Tree(
		repoPath string,
		dirPath string,
		provider tree.StatusProvider,
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

	NewWatcher(
		ctx context.Context,
		wsID string,
		repoPath string,
		base string,
		git watch.GitStatusProvider,
		dispatcher watch.Dispatcher,
	) *watch.Watcher

	NewFileWatcher(
		path string,
		emit func(change domain.FileChangeType),
	) *watch.FileWatcher
}

type fsEngine struct{}

func New() Engine {
	return &fsEngine{}
}

var _ Engine = (*fsEngine)(nil)

func (e *fsEngine) Tree(
	repoPath string,
	dirPath string,
	provider tree.StatusProvider,
) ([]domain.FileNode, error) {
	return tree.List(repoPath, dirPath, provider)
}

func (e *fsEngine) ReadContent(
	repoPath string,
	filePath string,
) (domain.FileContent, error) {
	return content.Read(repoPath, filePath)
}

func (e *fsEngine) WriteContent(
	repoPath string,
	filePath string,
	c string,
	encoding string,
) error {
	return content.Write(repoPath, filePath, c, encoding)
}

func (e *fsEngine) ReadHostContent(
	absPath string,
) (domain.FileContent, error) {
	return content.ReadAbs(absPath)
}

func (e *fsEngine) WriteHostContent(
	absPath string,
	c string,
	encoding string,
) error {
	return content.WriteAbs(absPath, c, encoding)
}

func (e *fsEngine) CreateFile(
	repoPath string,
	filePath string,
) error {
	return mutate.CreateFile(repoPath, filePath)
}

func (e *fsEngine) CreateDir(
	repoPath string,
	dirPath string,
) error {
	return mutate.CreateDir(repoPath, dirPath)
}

func (e *fsEngine) Copy(
	repoPath string,
	sourcePath string,
	destPath string,
) error {
	return mutate.Copy(repoPath, sourcePath, destPath)
}

func (e *fsEngine) Rename(
	repoPath string,
	oldPath string,
	newPath string,
) error {
	return mutate.Rename(repoPath, oldPath, newPath)
}

func (e *fsEngine) Delete(
	repoPath string,
	filePath string,
) error {
	return mutate.Delete(repoPath, filePath)
}

func (e *fsEngine) NewWatcher(
	_ context.Context,
	wsID string,
	repoPath string,
	base string,
	git watch.GitStatusProvider,
	dispatcher watch.Dispatcher,
) *watch.Watcher {
	return watch.NewWatcher(wsID, repoPath, base, git, dispatcher)
}

func (e *fsEngine) NewFileWatcher(
	path string,
	emit func(change domain.FileChangeType),
) *watch.FileWatcher {
	return watch.NewFileWatcher(path, emit)
}
