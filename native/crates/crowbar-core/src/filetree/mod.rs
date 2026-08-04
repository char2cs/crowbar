//! File-tree model — spec §4.2's `"file-tree model"` bucket of `crowbar-core`
//! (`native/mapping/tier-a-denominator.md` §5).
//!
//! [`gitignore`] is the first module ported into this area (item P3.76 —
//! `web/src/features/file-explorer/file-explorer/lib/file-tree-gitignore.ts`,
//! all 7 exports, confirmed LIVE at export level in §5's per-export table).
//!
//! A sibling item (`native/p3.75-core-filetree`) ports other §5 files
//! (`visible-file-tree-rows.ts`, `file-tree-git-status.ts`, etc.) into this
//! same module concurrently, in its own worktree. This file's module list is
//! kept minimal and additive on purpose so the two merge cleanly.

pub mod gitignore;

pub use gitignore::{
    FileTreeGitIgnoreRules, GitIgnoreFileContent, GitIgnoreFileReference, GitIgnoreTreeEntry,
    collect_git_ignore_file_references, create_file_tree_git_ignore_rules,
    is_path_git_ignored_by_file_tree_rules, is_workspace_relative_path,
};
