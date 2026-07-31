package folder

import "errors"

// ErrFolderCycle is returned when a move would put a folder inside its own
// subtree — under itself, under a folder it contains, or under a workspace filed
// inside it. The result is a set of rows unreachable from the repo root: they
// exist, nothing renders them, and nothing can drag them back out. The guard runs
// before any write, so the tree is never briefly cyclic. Handlers map it to 409.
var ErrFolderCycle = errors.New("usecases: a folder cannot be moved inside itself")

// ErrFolderCrossRepo is returned when a folder's parent, or the folder a
// workspace is filed under, belongs to a different repository. Folders are
// repo-scoped: the sidebar renders one repo's folders under one repo's header, so
// a cross-repo edge is a row that is simply never drawn. Handlers map it to 409.
var ErrFolderCrossRepo = errors.New("usecases: a folder and its parent must be in the same repository")

// ErrForkChainSplit is returned when filing a workspace into a folder would
// separate it from its fork parent. A workspace with a fork parent renders under
// that parent and inherits its folder from its fork ancestor — that is what makes
// the diff base, the merge eligibility and the reparent leaf-check agree with what
// is on screen. Only a FORK ROOT may carry a folder of its own, so moving a
// forked child into a folder is refused here rather than left to the UI to avoid.
// Handlers map it to 409.
var ErrForkChainSplit = errors.New("usecases: a workspace cannot be filed away from its fork parent")

// ErrFolderNameRequired is returned when a create or rename supplies a blank
// name. A nameless folder is an unlabelled box the user cannot tell apart from
// any other; it is refused at the usecase boundary so the API and any future
// caller share one rule. Handlers map it to 400.
var ErrFolderNameRequired = errors.New("usecases: folder name is required")
