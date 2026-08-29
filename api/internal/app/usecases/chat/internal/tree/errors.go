package tree

import "errors"

// ErrCycle is returned when a move would put a row inside its own subtree —
// under itself, under a folder it contains, or under a chat threaded off it. The
// result is a set of rows unreachable from the panel root: they exist, nothing
// renders them, and nothing can drag them back out. For a CHAT it is worse than
// unreachable: a chat's parent is what it reads, so a chat inside its own subtree
// is a context walk that never terminates at the root. The guard runs before any
// write, so the tree is never briefly cyclic. Handlers map it to 409.
var ErrCycle = errors.New("usecases: a chat or folder cannot be moved inside itself")

// ErrCrossWorkspace is returned when a CHAT's parent belongs to a different
// workspace. A folder carries no workspace and so can never trigger it — only a
// row threading under another CHAT can. A cross-workspace thread would mean
// reading turns from a workspace the user is not in. Handlers map it to 409.
var ErrCrossWorkspace = errors.New("usecases: a chat and its chat parent must be in the same workspace")

// ErrNameRequired is returned when a create or rename supplies a blank name. A
// nameless folder is an unlabelled box the user cannot tell apart from any
// other; it is refused at the usecase boundary so the API and any future caller
// share one rule. Handlers map it to 400.
var ErrNameRequired = errors.New("usecases: chat folder name is required")
