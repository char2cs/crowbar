package domain

// ChatType marks a row's kind in the sidebar forest: chat (conversation),
// branch (protected branch), or workflow. v0 only ever writes "chat";
// "workflow" is a forward-compat marker with no v0 writer (01 §2).
//
// ChatTypeFolder and ChatTypeBranch are no longer part of the closed
// taxonomy a Chat aggregate may be minted or retyped as
// (commands.validChatType) — a folder is a domain.Folder row now
// (2026-09-08 sidebar-placement-unification Task 5 for home-scoped, Task 8
// for repo-scoped too), and a workspace's own position is carried by its
// Node{Kind:workspace} row, never a retyped Chat proxy, since Task 9 deleted
// the machinery that minted/retyped one. Both constants stay defined here
// rather than deleted: a handful of read-side comparisons (a legacy row
// created before that machinery was retired, still legitimately typed this
// way on disk) still name them defensively, and neither can ever again match
// a freshly-minted Chat row's Type once nothing mints one.
type ChatType string

const (
	ChatTypeChat     ChatType = "chat"
	ChatTypeBranch   ChatType = "branch"
	ChatTypeFolder   ChatType = "folder"
	ChatTypeWorkflow ChatType = "workflow"
)
