//! File-tree model — spec §4.2's `"file-tree model"` bucket of
//! `crowbar-core`.
//!
//! Ported from the exports `native/mapping/tier-a-denominator.md` §8's
//! export-level "Deliverable 3 — scope recommendation" names for §5
//! ("File-tree model") — the export-level audit, not the file-level table
//! or prose, per that item's own warning that the prose has previously
//! described dead/test-only exports as though they were live:
//!
//! | module | ported from |
//! |---|---|
//! | [`types`] | `types/app.ts` (`AppFile`/`FileEntry`/`ContextMenuState`) |
//! | [`visible_rows`] | `lib/visible-file-tree-rows.ts` (5 of 6 exports — see below) |
//! | [`git_status`] | `lib/file-tree-git-status.ts` (all 6 exports, colour split — see [`git_status`]'s doc) |
//! | [`density`] | `lib/file-tree-density.ts` (4 of 6 exports, plus `rowHeight` only) |
//! | [`tree_utils`] | `utils/file-explorer-tree-utils.ts` (1 of 5 exports) |
//! | [`file_name`] | `../../file-system/controllers/file-utils.ts` (2 names, 1 function) |
//!
//! # What this item did not port, and why
//!
//! * **`visible-file-tree-rows.ts`'s `getStickyAncestorRow` (singular)** —
//!   §8's export-level audit found zero non-test callers; the plural
//!   [`visible_rows::get_sticky_ancestor_rows`] is what every real call site
//!   uses. See that module's doc for the one-line equivalence (call the
//!   plural, take `.last()`).
//! * **`file-tree-gitignore.ts`** — a separate item per this item's brief: it
//!   needs a Rust `ignore`-crate dependency decision
//!   (`native/mapping/tier-a-denominator.md` §8 recommends
//!   `ignore::gitignore::Gitignore`, one instance per directory, with the
//!   ancestor-first cascade reimplemented — no crate equivalent for that
//!   part) that this item does not make.
//! * **The 7 hook/store files** (`file-explorer-tree-store.ts`,
//!   `file-explorer-clipboard-store.ts`, and 5 `use-file-explorer-*` hooks,
//!   1,478 lines) — Phase-4 glue; GPUI's own action/keybinding and
//!   `Entity<T>` machinery replaces this wiring shape wholesale, matching
//!   `crate::keymap`'s precedent for the analogous hook layer.
//! * **`env-template.ts`, `file-upload.ts`** — CONDITIONAL, gated behind a
//!   specific right-click context-menu action; `file-upload.ts` is also
//!   `FileReader`/`<input type=file>`-bound platform code, not core logic.
//! * **`file-tree-api.ts`** — transport (`fetchFileTree`, `createFileNode`,
//!   ...); `crowbar-client` territory, not `crowbar-core`, per this item's
//!   brief.
//! * **`findFileInTree`** (`file-system/controllers/file-tree-utils.ts`) — a
//!   separate concern per this item's brief; left unported.
//! * **`file-explorer-tree-utils.ts`'s other four exports**
//!   (`filterHiddenFiles`, `addNewItemToTree`, `removeEditingItemsFromTree`,
//!   the exported `getAncestorDirectoryPaths`) — dead or test-only; see
//!   [`tree_utils`]'s module doc.
//! * **`file-tree-density.ts`'s `FILE_TREE_DENSITY_CONFIG.rowClassName` and
//!   `FILE_TREE_DENSITY_OPTIONS`** — presentation (Tailwind classes /
//!   Settings-tab dropdown copy); see [`density`]'s module doc.
//!
//! # The `colorClassName` split
//!
//! `file-tree-git-status.ts`'s `getFileTreeGitStatusDecoration` bundles a
//! hardcoded Tailwind `colorClassName` alongside its genuine
//! `statusLetter`/`label` classification. Per this item's brief, the colour
//! is ported as a separate, string-free enum ([`git_status::GitStatusColor`])
//! that a future `crowbar-ui` colour seal resolves — not carried as a
//! `String` in this crate. See [`git_status`]'s module doc for the full
//! reasoning.
//!
//! # `FileTreeDensity`: the settings/file-tree reconciliation
//!
//! `crate::settings::types::FileTreeDensity` was, until this item, a
//! deliberately-flagged narrow duplicate of this area's own type (that
//! module's doc said so explicitly, written before this item existed to
//! close the gap). [`density::FileTreeDensity`] is now the one definition;
//! `crate::settings::types` re-exports it. See [`density`]'s module doc.

pub mod density;
pub mod file_name;
pub mod git_status;
pub mod tree_utils;
pub mod types;
pub mod visible_rows;

pub use density::{
    DEFAULT_FILE_TREE_DENSITY, FileTreeDensity, is_file_tree_density, normalize_file_tree_density,
    row_height as file_tree_row_height,
};
pub use file_name::get_file_name;
pub use git_status::{
    FileTreeGitStatusDecoration, FileTreeGitStatusLookup, GitStatusColor,
    create_file_tree_git_status_lookup, get_file_tree_entry_git_status_decoration,
    get_file_tree_git_status_decoration, resolve_active_workspace_git_status,
};
pub use tree_utils::{ExplorerTargetBuffer, get_explorer_target_path};
pub use types::{AppFile, ContextMenuPosition, ContextMenuState, FileEntry};
pub use visible_rows::{
    BuildVisibleFileTreeRowsOptions, FileTreeSearchHit, FilterFileTreeForSearchResult,
    VisibleFileTreeRow, build_visible_file_tree_rows, compute_file_tree_search_hits,
    filter_file_tree_for_fff_hits, get_guide_ancestor_rows, get_sticky_ancestor_rows,
};
