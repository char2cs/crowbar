package file

import (
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

type fsEngineAdapter struct {
	enginefs.Engine
}

// NewEngineAdapter wraps the filesystem engine so its Tree method accepts the
// file package's FileStatusProvider, satisfying the FsEngine surface the file
// usecase consumes.
func NewEngineAdapter(
	engine enginefs.Engine,
) FsEngine {
	return &fsEngineAdapter{Engine: engine}
}

func (a *fsEngineAdapter) Tree(
	repoPath string,
	dirPath string,
	provider FileStatusProvider,
) ([]domain.FileNode, error) {
	return a.Engine.Tree(repoPath, dirPath, provider)
}
