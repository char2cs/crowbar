package usecases

import (
	"github.com/char2cs/crowbar/api/internal/app/usecases/file"
	"github.com/char2cs/crowbar/api/internal/domain"
	enginefs "github.com/char2cs/crowbar/api/internal/engine/fs"
)

type fsEngineAdapter struct {
	enginefs.Engine
}

// newFsEngineAdapter wraps the filesystem engine so its Tree method accepts the
// file package's FileStatusProvider, satisfying the file.FsEngine surface the
// file usecase consumes.
func newFsEngineAdapter(
	engine enginefs.Engine,
) file.FsEngine {
	return &fsEngineAdapter{Engine: engine}
}

func (a *fsEngineAdapter) Tree(
	repoPath string,
	dirPath string,
	provider file.FileStatusProvider,
) ([]domain.FileNode, error) {
	return a.Engine.Tree(repoPath, dirPath, provider)
}
