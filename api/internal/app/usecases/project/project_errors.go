package project

import (
	"errors"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

var ErrFolderNotFound = problem.New(problem.NotFound, "folder does not exist")

var ErrRepoAlreadyImported = problem.New(problem.Conflict, "this folder is already added as a repository")

var ErrNoNodesWired = errors.New("project import: no node surface wired; refusing to create a row with no position")

var errNoCrowbarHome = errors.New("crowbar home is not set")
