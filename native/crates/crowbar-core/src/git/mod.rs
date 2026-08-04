//! Git model — spec §4.2's `"git model"` bucket of `crowbar-core`.
//!
//! Ported from the six files
//! `native/mapping/tier-a-denominator.md` §"Git model" names as the
//! genuine, portable git-model logic (of `web/src/features/git/`'s ~1,190
//! lines of non-component git/diff logic, the rest is either duplicated by
//! `crowbar-proto`'s already-generated DTOs, belongs to `crowbar-diff`'s own
//! logic partition per §4.2/§12 — windowing, search, placeholder-hunk
//! geometry — or is a caching shim D6 deletes):
//!
//! | module | ported from |
//! |---|---|
//! | [`git_status_to_changed_files`] | `utils/git-status-to-changed-files.ts` |
//! | [`build_git_folder_tree`] | `utils/build-git-folder-tree.ts` |
//! | [`review_file_summary_to_git_diff`] | `utils/review-file-summary-to-git-diff.ts` |
//! | [`normalize_diff`] | `utils/normalize-diff.ts` (see its module doc — ported as an invariant + regression test, not two pass-through functions) |
//! | [`diff_buffer_path`] | `utils/diff-buffer-path.ts` |
//! | [`branch_action`] | `lib/branch-action.ts` |
//! | [`get_file_status`] | `utils/git-diff-helpers.ts` (P3.79 — see that module's doc for why it lands here and not `crowbar-diff`, despite sharing a source file with the `crowbar-diff`-bound placeholder-hunk algebra) |
//!
//! # Types: what was reused from `crowbar-proto`, what was hand-rolled
//!
//! `crowbar-proto` already generates DTOs matching most of
//! `web/src/features/git/types/git-types.ts` (§9.2) — the survey calls this
//! out explicitly: the hand-written TS type file is not new Tier A work,
//! it duplicates generated output. Reused directly, following
//! `crate::workspace`'s own precedent (`placeholder.rs`/`branch.rs` reusing
//! `WorkspaceDTO`) of preferring a generated DTO over a hand-rolled
//! duplicate whenever the fields actually match:
//!
//! * [`crowbar_proto::api_v0_dto::GitFileDTO`] for TS `GitFile` (`path`,
//!   `status`, `staged` line up exactly).
//! * [`crowbar_proto::domain_git::ReviewFileSummary`] and
//!   [`crowbar_proto::domain_git::GitFileStatus`] for the review-summary
//!   input, unchanged.
//! * [`crowbar_proto::domain_git::DiffLine`] as the element type of
//!   [`types::GitDiff::lines`] (always empty in this item's outputs, but no
//!   reason to duplicate the line type when nothing here diverges from it).
//!
//! **Not** reused: [`crowbar_proto::domain_git::FileDiff`], for TS
//! `GitDiff`. Its `additions`/`deletions`/`uncommitted` are non-optional,
//! but `git_status_to_changed_files`'s source data carries no counts at all
//! — forcing it through `FileDiff` would silently invent a `0` for a fact
//! the daemon never reported. [`types::GitDiff`] is hand-rolled instead,
//! with those three fields kept `Option`-typed, so it stays the ONE type
//! both projection functions can share — matching what the TS source
//! actually does (`GitDiff` is used, unmodified, by both `.ts` files) rather
//! than splitting into two incompatible shapes. See `types`'s module doc for
//! the full reasoning, and which `GitDiff` fields were left out entirely
//! (none of this item's functions ever set `new_path`/`is_binary`/
//! `is_image`/`old_blob_base64`/`new_blob_base64`/`raw_patch`, so they are
//! not ported — a field with no producer and no test is the same
//! "declaration, not behaviour" problem as an untested function).
//!
//! `MultiFileDiff` (TS `git-diff-types.ts`) is not ported at all: nothing in
//! this item's six functions constructs or consumes one — see
//! [`normalize_diff`]'s module doc, whose tests exercise
//! `crowbar_proto::domain_git::MultiFileDiff` directly instead. `ParsedHunk`
//! (same TS file) is dead code upstream — declared, never constructed
//! anywhere in `web/src` — and is not ported, matching this port's stated
//! preference for live code paths.

pub mod branch_action;
pub mod build_git_folder_tree;
pub mod diff_buffer_path;
pub mod get_file_status;
pub mod git_status_to_changed_files;
pub mod normalize_diff;
pub mod review_file_summary_to_git_diff;
pub mod types;

pub use branch_action::{
    BranchAction, BranchActionInput, BranchActionKind, RemoteAction, resolve_branch_action,
};
pub use build_git_folder_tree::{
    GitFolderNode, build_git_folder_tree, collect_node_files, create_folder_node,
    normalize_path_segments, sort_files_by_path, sort_folders_by_name,
};
pub use diff_buffer_path::get_diff_buffer_file_path;
pub use get_file_status::get_file_status;
pub use git_status_to_changed_files::git_status_to_changed_files;
pub use review_file_summary_to_git_diff::review_files_summary_to_changed_files;
pub use types::GitDiff;

/// Re-exported for `crowbar-diff`, whose own §4.2 dependency contract
/// (`native/README.md` / spec §4.2's crate-contracts table) is `ui`/
/// `state`/`core` — no direct `crowbar-proto` edge. That crate's ported
/// placeholder-hunk-geometry algebra
/// (`native/mapping/tier-a-denominator.md` §2, P3.79) is typed directly
/// against these three DTOs rather than against `@pierre/diffs`'s `Hunk`/
/// `FileDiffMetadata` (see `crowbar_diff::review_placeholder`'s module doc
/// for the full retyping rationale and where the shapes diverge). Passing
/// them through `crowbar-core` — which already depends on `crowbar-proto`
/// directly — is the same pattern `crowbar-ui` already uses for `gpui`
/// (`pub use gpui;`, `native/README.md`): one crate becomes the sanctioned
/// gateway so a framework/DTO-generator bump is one edit here, not one per
/// leaf crate.
pub use crowbar_proto::domain_git::{FileDiff, FileOutline, HunkShape};
