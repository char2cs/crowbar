package agentchatfolder

import "errors"

// ErrCycle is returned when a move would put a row inside its own subtree —
// under itself, under a folder it contains, or under a chat threaded off it. The
// result is a set of rows unreachable from the panel root: they exist, nothing
// renders them, and nothing can drag them back out. For a CHAT it is worse than
// unreachable: a chat's parent is what it reads, so a chat inside its own subtree
// is a context walk that never terminates at the root. The guard runs before any
// write, so the tree is never briefly cyclic. Handlers map it to 409.
var ErrCycle = errors.New("usecases: a chat or folder cannot be moved inside itself")

// ErrCrossWorkspace is returned when a row's parent belongs to a different
// workspace. Chats and their folders are workspace-scoped: the panel renders one
// workspace's tree, so a cross-workspace edge is a row that is simply never
// drawn — and for a chat it would additionally mean reading turns from a
// workspace the user is not in. Handlers map it to 409.
var ErrCrossWorkspace = errors.New("usecases: a chat or folder and its parent must be in the same workspace")

// ErrNameRequired is returned when a create or rename supplies a blank name. A
// nameless folder is an unlabelled box the user cannot tell apart from any
// other; it is refused at the usecase boundary so the API and any future caller
// share one rule. Handlers map it to 400.
var ErrNameRequired = errors.New("usecases: chat folder name is required")
