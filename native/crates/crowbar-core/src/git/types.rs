//! The unified sidebar-tree projection type that
//! [`super::git_status_to_changed_files`] and
//! [`super::review_file_summary_to_git_diff`] both produce.
//!
//! Ported from the fields of `web/src/features/git/types/git-types.ts`'s
//! `GitDiff` that this item's six files actually populate: `file_path`,
//! `old_path`, `is_new`, `is_deleted`, `is_renamed`, `additions`,
//! `deletions`, `uncommitted`, `lines`. **Not** ported: `new_path`,
//! `is_binary`, `is_image`, `old_blob_base64`, `new_blob_base64`,
//! `raw_patch` — no function in this item's scope ever sets or reads them,
//! and a field with no producer and no test would be exactly the
//! "declaration, not behaviour" shape this port avoids (see
//! `crate::workspace::scope`'s test module for the same principle applied to
//! a test rather than a field).
//!
//! # Why this is hand-rolled rather than `crowbar_proto::domain_git::FileDiff`
//!
//! `crowbar-proto` already generates [`crowbar_proto::domain_git::FileDiff`]
//! with an almost-identical field set — `native/mapping/tier-a-denominator.md`
//! §"Git model" notes the overlap explicitly, and `crate::workspace`'s own
//! modules (`placeholder.rs`, `branch.rs`) set the precedent of reusing a
//! generated DTO instead of hand-rolling a duplicate whenever the fields
//! actually line up.
//!
//! They don't line up here. `FileDiff.additions` / `.deletions` /
//! `.uncommitted` are **non-optional** — once the daemon has produced a
//! `FileDiff` it always knows these (a `-1` sentinel marks "binary", never
//! "absent"). But `gitStatusToChangedFiles`'s source data
//! ([`crowbar_proto::api_v0_dto::GitFileDTO`], the raw working-tree status)
//! carries no counts at all — not even a sentinel. Forcing its output through
//! `FileDiff` would mean inventing a value (`0`) for a fact the daemon never
//! reported: a silent behaviour change from the TS source, whose
//! `additions?`/`deletions?`/`uncommitted?` are genuinely optional and left
//! `undefined` here — asserted directly by the TS test suite
//! (`git-status-to-changed-files.test.ts`, `'carries no per-file line counts
//! or uncommitted flag (unavailable from status)'`).
//!
//! `GitDiff` below keeps those three fields `Option`-typed for the same
//! reason the TS type does: it is the ONE shape both projections were
//! designed to share — both TS files say, verbatim, in their own doc
//! comments, that they project into "the `GitDiff[]` shape the sidebar
//! `ChangedFilesTree` consumes." Defaulting `git_status_to_changed_files`'s
//! output to `FileDiff` would have split that shared contract into two
//! Rust types with no common consumer, which is precisely what the TS design
//! avoids by keeping the three fields optional on one type.

use crowbar_proto::domain_git::DiffLine;

/// One file's sidebar-tree projection. See the module docs for which
/// `GitDiff` fields were ported and which were deliberately left out.
#[derive(Debug, Clone, PartialEq, Default)]
pub struct GitDiff {
    pub file_path: String,
    pub old_path: Option<String>,
    pub is_new: bool,
    pub is_deleted: bool,
    pub is_renamed: bool,
    /// `None` when the source data carries no per-file line counts (the raw
    /// working-tree status). `Some(-1)` marks a binary file, matching the
    /// wire convention `ReviewFileSummary`'s own doc comment states.
    pub additions: Option<i64>,
    pub deletions: Option<i64>,
    /// `None` when the source data cannot say either way. `Some(false)` is a
    /// real, asserted fact ("this file has no uncommitted working-tree
    /// changes"), not "unknown" — see
    /// `review_file_summary_to_git_diff`'s doc comment for the case that
    /// produces it.
    pub uncommitted: Option<bool>,
    pub lines: Vec<DiffLine>,
}
