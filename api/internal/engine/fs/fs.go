// Package fs is the filesystem engine (05). Stateless per call; the watcher
// is the one stateful per-workspace resource.
package fs

import (
	"context"

	"github.com/char2cs/crowbar/api/internal/domain"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/content"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/mutate"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/tree"
	"github.com/char2cs/crowbar/api/internal/engine/fs/internal/watch"
)

// Engine is the full filesystem operation surface (05).
type Engine interface {
	// Tree returns one level of the file tree rooted at dirPath (05 §2).
	// dirPath is relative to repoPath; empty string = root.
	Tree(
		repoPath string,
		dirPath string,
		provider tree.StatusProvider,
	) ([]domain.FileNode, error)

	// ReadContent returns the content of a file (05 §3).
	ReadContent(
		repoPath string,
		filePath string,
	) (domain.FileContent, error)

	// WriteContent writes content to a file (05 §3). encoding is "base64" for a
	// byte-faithful binary payload or "" / "utf8" for raw UTF-8.
	WriteContent(
		repoPath string,
		filePath string,
		content string,
		encoding string,
	) error

	// CreateFile creates an empty file (05 §4).
	CreateFile(
		repoPath string,
		filePath string,
	) error

	// CreateDir creates a directory (05 §4).
	CreateDir(
		repoPath string,
		dirPath string,
	) error

	// Copy duplicates a file or directory byte-faithfully (05 §4). Directories
	// copy recursively; the destination must not already exist.
	Copy(
		repoPath string,
		sourcePath string,
		destPath string,
	) error

	// Rename renames or moves a file or directory (05 §4).
	Rename(
		repoPath string,
		oldPath string,
		newPath string,
	) error

	// Delete removes a file or directory (05 §4).
	Delete(
		repoPath string,
		filePath string,
	) error

	// NewWatcher builds a file watcher for a workspace. The caller owns Start
	// and Stop (05 §5, 03 §6).
	NewWatcher(
		ctx context.Context,
		wsID string,
		repoPath string,
		base string,
		git watch.GitStatusProvider,
		dispatcher watch.Dispatcher,
	) *watch.Watcher
}

type fsEngine struct{}

// New returns a new Engine.
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
