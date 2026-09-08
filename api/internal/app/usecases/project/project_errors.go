package project

import "errors"

// ErrFolderNotFound is returned when a project import targets a path that does
// not exist on disk. The import validates the path before persisting anything,
// so a nonexistent folder leaves no project behind. Handlers map it to HTTP
// 404 via libs.StatusAndMessage; the message stays free of filesystem
// internals on purpose.
var ErrFolderNotFound = errors.New("folder does not exist")

// ErrRepoAlreadyImported is returned when a repo folder another project has
// already imported is added again. A branch can be checked out in only one
// worktree, so two projects owning one folder cannot both manage its protected
// branches: whichever imports second gets a placeholder for every branch the
// first already claimed — a repository that looks imported but can manage
// nothing. One folder therefore belongs to exactly one project. Handlers map it
// to HTTP 409 via libs.StatusAndMessage, and it is wrapped with the name of the
// project that already has the folder so the message says where to look.
var ErrRepoAlreadyImported = errors.New("this folder is already added as a repository")

// ErrNoNodesWired is returned when a repo import runs on a usecase whose Node
// surface (NodePlacements) was never wired.
//
// It refuses rather than silently persisting a repo with no position row —
// mirrors ErrNoOwningChats's own reasoning: an unwired daemon fails at its
// first import instead of producing a repo whose sidebar entry every densify
// pass silently skips.
var ErrNoNodesWired = errors.New("project import: no node surface wired; refusing to create a repo with no position")
