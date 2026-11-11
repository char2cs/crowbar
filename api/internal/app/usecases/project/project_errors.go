package project

import (
	"errors"

	"github.com/char2cs/crowbar/api/internal/domain/problem"
)

// ErrFolderNotFound is returned when a project import targets a path that does not
// exist; it is checked before anything persists. The message omits filesystem internals.
var ErrFolderNotFound = problem.New(problem.NotFound, "folder does not exist")

// ErrRepoAlreadyImported is returned when a folder another project imported is added
// again: a branch checks out in only one worktree, so the second project could manage
// nothing. It is wrapped with the owning project's name so the message says where to look.
var ErrRepoAlreadyImported = problem.New(problem.Conflict, "this folder is already added as a repository")

// ErrNoNodesWired is returned when an import runs without a Node surface wired. It
// refuses rather than persisting a repo or workspace row with no sidebar position,
// which every densify pass would silently skip.
var ErrNoNodesWired = errors.New("project import: no node surface wired; refusing to create a row with no position")

// errNoCrowbarHome is why a protected branch gets no managed worktree when
// the home resolves to nothing.
var errNoCrowbarHome = errors.New("crowbar home is not set")
