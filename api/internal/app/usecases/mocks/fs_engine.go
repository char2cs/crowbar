package mocks

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
)

// FsEngine is a fake of the filesystem-engine surface used by the file usecase.
type FsEngine struct {
	TreeFn func(repoPath, dirPath string, provider file.FileStatusProvider) ([]domain.FileNode, error)

	ReadContentFn      func(repoPath, filePath string) (domain.FileContent, error)
	WriteContentFn     func(repoPath, filePath, content, encoding string) error
	ReadHostContentFn  func(absPath string) (domain.FileContent, error)
	WriteHostContentFn func(absPath, content, encoding string) error
	CreateFileFn       func(repoPath, filePath string) error
	CreateDirFn        func(repoPath, dirPath string) error
	CopyFn             func(repoPath, sourcePath, destPath string) error
	RenameFn           func(repoPath, oldPath, newPath string) error
	DeleteFn           func(repoPath, filePath string) error
}

// NewFsEngine returns an empty FsEngine.
func NewFsEngine() *FsEngine {
	return &FsEngine{}
}

func (e *FsEngine) Tree(
	repoPath string,
	dirPath string,
	provider file.FileStatusProvider,
) ([]domain.FileNode, error) {
	return e.TreeFn(repoPath, dirPath, provider)
}

func (e *FsEngine) ReadContent(
	repoPath string,
	filePath string,
) (domain.FileContent, error) {
	return e.ReadContentFn(repoPath, filePath)
}

func (e *FsEngine) WriteContent(
	repoPath string,
	filePath string,
	content string,
	encoding string,
) error {
	return e.WriteContentFn(repoPath, filePath, content, encoding)
}

func (e *FsEngine) ReadHostContent(
	absPath string,
) (domain.FileContent, error) {
	return e.ReadHostContentFn(absPath)
}

func (e *FsEngine) WriteHostContent(
	absPath string,
	content string,
	encoding string,
) error {
	return e.WriteHostContentFn(absPath, content, encoding)
}

func (e *FsEngine) CreateFile(
	repoPath string,
	filePath string,
) error {
	return e.CreateFileFn(repoPath, filePath)
}

func (e *FsEngine) CreateDir(
	repoPath string,
	dirPath string,
) error {
	return e.CreateDirFn(repoPath, dirPath)
}

func (e *FsEngine) Copy(
	repoPath string,
	sourcePath string,
	destPath string,
) error {
	return e.CopyFn(repoPath, sourcePath, destPath)
}

func (e *FsEngine) Rename(
	repoPath string,
	oldPath string,
	newPath string,
) error {
	return e.RenameFn(repoPath, oldPath, newPath)
}

func (e *FsEngine) Delete(
	repoPath string,
	filePath string,
) error {
	return e.DeleteFn(repoPath, filePath)
}
