package domain

// ChatType marks a row's kind in the sidebar forest: chat (conversation),
// branch (protected branch), or workflow. v0 only ever writes "chat";
// "workflow" is a forward-compat marker with no v0 writer (01 §2).
//
// ChatTypeFolder is no longer part of the closed taxonomy a Chat aggregate
// may be minted or retyped as (commands.validChatType) — a folder is a
// domain.Folder row now (2026-09-08 sidebar-placement-unification Task 5 for
// home-scoped, Task 8 for repo-scoped too). The constant itself stays
// defined here rather than deleted: a handful of read-side comparisons
// (e.g. "is this row a folder, exclude it") still name it defensively, and
// it can never again match a live Chat row's Type once nothing mints one —
// deleting it outright is Task 9/11's cleanup sweep, alongside ChatTypeBranch.
type ChatType string

const (
	ChatTypeChat     ChatType = "chat"
	ChatTypeBranch   ChatType = "branch"
	ChatTypeFolder   ChatType = "folder"
	ChatTypeWorkflow ChatType = "workflow"
)
