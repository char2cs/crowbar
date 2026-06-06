package usecases

import (
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

type fsEngineAdapter struct {
	enginefs.Engine
}

func newFsEngineAdapter(
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
