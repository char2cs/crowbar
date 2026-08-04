//! Flatten-with-expand-state, plus search/filter with ancestor expansion —
//! the substantive file-tree-model algorithm.
//!
//! Ported from `web/src/features/file-explorer/file-explorer/lib/
//! visible-file-tree-rows.ts` (238 lines). Five of its exports are ported;
//! the sixth, the singular `getStickyAncestorRow`, is not — see
//! [`get_sticky_ancestor_rows`]'s doc for why, and how to get the same
//! answer from the plural this module does export.
//!
//! # The real branches, enumerated before writing any Rust
//!
//! * **A collapsed ancestor hides everything below it.**
//!   [`build_visible_file_tree_rows`]'s recursion only descends into a
//!   directory's `children` when that directory's own path is in
//!   `expanded_paths` — a descendant three levels deep behind one collapsed
//!   ancestor never gets a row, regardless of whether ITS OWN path is also
//!   in `expanded_paths`. `hides_nested_descendants_when_a_middle_folder_collapses`
//!   below is the ported TS test proving exactly this: expanding
//!   `/root/src/features` has no effect while `/root/src` itself is
//!   collapsed.
//! * **An empty (or whitespace-only) search query is a no-op, not "match
//!   nothing has changed since the last query."** [`compute_file_tree_search_hits`]
//!   returns `[]` immediately for `query.trim().is_empty()` — it never walks
//!   the tree at all for that input, matching the TS source's own early
//!   `if (!q) return []`.
//! * **A match's ancestors are re-added even when the ancestors themselves
//!   don't match the query.** [`filter_file_tree_for_fff_hits`]'s recursion
//!   keeps a non-matching directory in the output whenever at least one of
//!   its (possibly deeply nested) descendants matched — `matching_children`
//!   being non-empty is enough, `is_match` is not required. An ancestor
//!   that is EXCLUDED (a sibling subtree with no match in it) is dropped
//!   from the output tree entirely, not merely left uninspected — pruned
//!   subtrees never surface as empty directories.
//! * **A matched directory does not auto-expand its own non-matching
//!   children.** `expanded_paths` only gains a directory's path when
//!   `matching_children` is non-empty — a matched *folder itself* (with no
//!   matching descendant) is kept in the output tree (so it still renders)
//!   but is not added to `expanded_paths`, so its own unmatched children
//!   stay hidden. `keeps_a_matched_folder_without_expanding_unmatched_descendants`
//!   below is the ported TS test for this.
//! * **An empty hit list returns a genuinely empty result, not "the whole
//!   tree, nothing highlighted."** `filterFileTreeForFffHits(files, [])`
//!   short-circuits before touching `files` at all — `files: []`, not the
//!   original list unfiltered. This is deliberate: the UI's "press Enter in
//!   the fff picker with zero results" state should show nothing, not
//!   silently fall back to the unfiltered tree.

use std::collections::{HashMap, HashSet};

use super::types::FileEntry;

/// One flattened, visible row of the tree — `depth` is levels below the
/// tree's own roots (root-level entries are depth `0`).
#[derive(Debug, Clone, PartialEq)]
pub struct VisibleFileTreeRow {
    pub file: FileEntry,
    pub depth: usize,
    pub is_expanded: bool,
    /// Set only when `compact_folders` collapsed a chain of single-child
    /// directories into this one row — `"parent/child/grandchild"` rather
    /// than the leaf's own bare name.
    pub display_name: Option<String>,
}

/// Mirrors `BuildVisibleFileTreeRowsOptions`.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct BuildVisibleFileTreeRowsOptions {
    pub compact_folders: bool,
}

/// Mirrors `FilterFileTreeForSearchResult`.
#[derive(Debug, Clone, PartialEq, Default)]
pub struct FilterFileTreeForSearchResult {
    pub files: Vec<FileEntry>,
    pub expanded_paths: HashSet<String>,
    pub matched_paths: HashSet<String>,
    /// Matched paths in the order their originating [`FileTreeSearchHit`]
    /// was supplied — a hit whose path never actually appears in the tree
    /// contributes nothing, and a path hit twice contributes once, at its
    /// first occurrence's position.
    pub ordered_matched_paths: Vec<String>,
    pub match_count: usize,
}

/// Mirrors `FileTreeSearchHit`.
#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct FileTreeSearchHit {
    pub path: String,
}

/// Mirrors `getCompactFolderChild` (module-private in the TS source too).
///
/// A directory only compacts into its single child when BOTH are plain
/// directories with no in-progress UI state (not editing/renaming/a new-item
/// placeholder) — an in-progress rename mid-chain must interrupt the compact
/// walk so the user can see (and finish editing) that exact row.
fn get_compact_folder_child(item: &FileEntry) -> Option<&FileEntry> {
    if !item.is_dir || item.is_editing || item.is_renaming || item.is_new_item {
        return None;
    }
    let children = item.children.as_ref()?;
    if children.len() != 1 {
        return None;
    }
    let child = &children[0];
    if !child.is_dir || child.is_editing || child.is_renaming || child.is_new_item {
        return None;
    }
    Some(child)
}

/// Mirrors `buildVisibleFileTreeRows`. See this module's doc for the
/// branches this walk actually has — in particular, what a collapsed
/// ancestor does to a matching/expanded descendant.
///
/// `clippy::implicit_hasher` (pedantic) suggests generalising
/// `expanded_paths` over `S: BuildHasher` so a caller could plug in a
/// non-default hasher. Nothing in this crate (or its one intended caller,
/// the eventual GPUI file-tree view) has a reason to use anything but the
/// standard hasher for a per-render expand-state set — the generic
/// parameter would be pure API noise with no real caller ever supplying
/// anything else.
#[must_use]
#[allow(clippy::implicit_hasher)]
pub fn build_visible_file_tree_rows(
    files: &[FileEntry],
    expanded_paths: &HashSet<String>,
    options: BuildVisibleFileTreeRowsOptions,
) -> Vec<VisibleFileTreeRow> {
    let mut rows = Vec::new();
    walk_visible_rows(files, 0, expanded_paths, options.compact_folders, &mut rows);
    rows
}

fn walk_visible_rows(
    items: &[FileEntry],
    depth: usize,
    expanded_paths: &HashSet<String>,
    compact_folders: bool,
    rows: &mut Vec<VisibleFileTreeRow>,
) {
    for item in items {
        let mut row_file = item;
        let mut display_name_parts = vec![item.name.clone()];

        if compact_folders {
            while expanded_paths.contains(&row_file.path) {
                let Some(child) = get_compact_folder_child(row_file) else {
                    break;
                };
                row_file = child;
                display_name_parts.push(child.name.clone());
            }
        }

        // A brand-new inline-edit placeholder (is_new_item) must never count
        // as an expanded directory — its path can be '' (the root), which
        // would otherwise load the workspace root as its children and
        // duplicate the whole tree.
        let is_expanded =
            row_file.is_dir && !row_file.is_new_item && expanded_paths.contains(&row_file.path);

        rows.push(VisibleFileTreeRow {
            file: row_file.clone(),
            depth,
            is_expanded,
            display_name: if display_name_parts.len() > 1 {
                Some(display_name_parts.join("/"))
            } else {
                None
            },
        });

        if row_file.is_dir
            && is_expanded
            && let Some(children) = &row_file.children
        {
            walk_visible_rows(children, depth + 1, expanded_paths, compact_folders, rows);
        }
    }
}

/// Mirrors `normalizeSearchPath` (module-private in the TS source too).
fn normalize_search_path(path: &str) -> String {
    let normalized = path.replace('\\', "/");
    if normalized == "/" {
        return normalized;
    }
    normalized.trim_end_matches('/').to_string()
}

/// Mirrors `computeFileTreeSearchHits`.
///
/// Every loaded node whose NAME contains `query` (case-insensitive
/// substring). Matches both files and directories; feed the result to
/// [`filter_file_tree_for_fff_hits`] to prune the tree to the matches and
/// their ancestors. Skips in-progress inline-edit placeholders. Only loaded
/// levels are searched — the tree is lazy, so an unexpanded directory's
/// children (never loaded into `files`) can't be searched.
#[must_use]
pub fn compute_file_tree_search_hits(files: &[FileEntry], query: &str) -> Vec<FileTreeSearchHit> {
    let q = query.trim().to_lowercase();
    if q.is_empty() {
        return Vec::new();
    }
    let mut hits = Vec::new();
    walk_search_hits(files, &q, &mut hits);
    hits
}

fn walk_search_hits(items: &[FileEntry], q: &str, hits: &mut Vec<FileTreeSearchHit>) {
    for item in items {
        if item.is_new_item || item.is_editing {
            continue;
        }
        if item.name.to_lowercase().contains(q) {
            hits.push(FileTreeSearchHit {
                path: item.path.clone(),
            });
        }
        if let Some(children) = &item.children {
            walk_search_hits(children, q, hits);
        }
    }
}

/// Mirrors `filterFileTreeForFffHits`. See this module's doc for the
/// ancestor-expansion branches — what happens to a match's non-matching
/// ancestors, and what an empty `hits` list produces.
#[must_use]
pub fn filter_file_tree_for_fff_hits(
    files: &[FileEntry],
    hits: &[FileTreeSearchHit],
) -> FilterFileTreeForSearchResult {
    let hit_paths: Vec<String> = hits
        .iter()
        .map(|h| normalize_search_path(&h.path))
        .collect();
    let hit_path_set: HashSet<&str> = hit_paths.iter().map(String::as_str).collect();

    if hit_path_set.is_empty() {
        return FilterFileTreeForSearchResult::default();
    }

    let mut expanded_paths = HashSet::new();
    let mut matched_paths = HashSet::new();
    let mut matched_tree_path_by_hit_path: HashMap<String, String> = HashMap::new();

    let filtered_files = walk_filter_for_hits(
        files,
        &hit_path_set,
        &mut expanded_paths,
        &mut matched_paths,
        &mut matched_tree_path_by_hit_path,
    );

    let mut ordered_matched_paths = Vec::new();
    let mut seen_ordered_paths = HashSet::new();
    for hit_path in &hit_paths {
        if let Some(tree_path) = matched_tree_path_by_hit_path.get(hit_path)
            && seen_ordered_paths.insert(tree_path.clone())
        {
            ordered_matched_paths.push(tree_path.clone());
        }
    }

    let match_count = matched_paths.len();
    FilterFileTreeForSearchResult {
        files: filtered_files,
        expanded_paths,
        matched_paths,
        ordered_matched_paths,
        match_count,
    }
}

fn walk_filter_for_hits(
    items: &[FileEntry],
    hit_path_set: &HashSet<&str>,
    expanded_paths: &mut HashSet<String>,
    matched_paths: &mut HashSet<String>,
    matched_tree_path_by_hit_path: &mut HashMap<String, String>,
) -> Vec<FileEntry> {
    let mut out = Vec::new();
    for item in items {
        let matching_children = match &item.children {
            Some(children) => walk_filter_for_hits(
                children,
                hit_path_set,
                expanded_paths,
                matched_paths,
                matched_tree_path_by_hit_path,
            ),
            None => Vec::new(),
        };
        let normalized_path = normalize_search_path(&item.path);
        let is_match = hit_path_set.contains(normalized_path.as_str());

        if !is_match && matching_children.is_empty() {
            continue;
        }

        if is_match {
            matched_paths.insert(item.path.clone());
            matched_tree_path_by_hit_path.insert(normalized_path, item.path.clone());
        }

        if item.is_dir && !matching_children.is_empty() {
            expanded_paths.insert(item.path.clone());
        }

        let mut kept = item.clone();
        kept.children = if matching_children.is_empty() {
            item.children.clone()
        } else {
            Some(matching_children)
        };
        out.push(kept);
    }
    out
}

/// The last element of [`get_sticky_ancestor_rows`] — the nearest sticky
/// ancestor for a row at `first_visible_index`, or `None` at depth `0`.
///
/// TS's `getStickyAncestorRow` (singular) is not ported as a separate
/// function: `native/mapping/tier-a-denominator.md` §8's export-level audit
/// found it has zero non-test callers anywhere in `web/src` — the plural
/// [`get_sticky_ancestor_rows`] is what `file-explorer-tree.tsx` actually
/// calls, 7 real call sites. The singular's own TS body is
/// `getStickyAncestorRows(rows, firstVisibleIndex)[length - 1] ?? null` —
/// exactly what calling the plural and taking its last element gives you, so
/// there is nothing this function would add over calling
/// [`get_sticky_ancestor_rows`] directly and reading `.last()`. Kept here
/// only as a doc-comment pointer for anyone who came looking for the
/// singular name; it is not part of this module's public API.
#[cfg(test)]
fn sticky_ancestor_row_via_plural(
    rows: &[VisibleFileTreeRow],
    first_visible_index: usize,
) -> Option<VisibleFileTreeRow> {
    get_sticky_ancestor_rows(rows, first_visible_index)
        .last()
        .cloned()
}

/// Mirrors `getStickyAncestorRows` (plural) — the full ancestor stack for
/// the row at `first_visible_index`, root-first, one entry per depth level
/// strictly above it. Empty at depth `0` (nothing is above a root row).
#[must_use]
pub fn get_sticky_ancestor_rows(
    rows: &[VisibleFileTreeRow],
    first_visible_index: usize,
) -> Vec<VisibleFileTreeRow> {
    let Some(first_visible_row) = rows.get(first_visible_index) else {
        return Vec::new();
    };
    if first_visible_row.depth == 0 {
        return Vec::new();
    }

    let mut ancestors: Vec<Option<VisibleFileTreeRow>> = vec![None; first_visible_row.depth];
    let mut remaining = first_visible_row.depth;

    let mut index = first_visible_index;
    while remaining > 0 && index > 0 {
        index -= 1;
        let candidate = &rows[index];
        if candidate.depth < first_visible_row.depth && ancestors[candidate.depth].is_none() {
            ancestors[candidate.depth] = Some(candidate.clone());
            remaining -= 1;
        }
    }

    ancestors.into_iter().flatten().collect()
}

/// Mirrors `getGuideAncestorRows` — like [`get_sticky_ancestor_rows`], but
/// keeps the `None` gaps for depth levels whose ancestor row wasn't found
/// (the indent-guide renderer needs a stable per-depth slot, not a
/// compacted list).
#[must_use]
pub fn get_guide_ancestor_rows(
    rows: &[VisibleFileTreeRow],
    row_index: usize,
) -> Vec<Option<VisibleFileTreeRow>> {
    let Some(row) = rows.get(row_index) else {
        return Vec::new();
    };
    if row.depth == 0 {
        return Vec::new();
    }

    let mut ancestors: Vec<Option<VisibleFileTreeRow>> = vec![None; row.depth];
    let mut remaining = row.depth;

    let mut index = row_index;
    while remaining > 0 && index > 0 {
        index -= 1;
        let candidate = &rows[index];
        if candidate.depth < row.depth && ancestors[candidate.depth].is_none() {
            ancestors[candidate.depth] = Some(candidate.clone());
            remaining -= 1;
        }
    }

    ancestors
}

#[cfg(test)]
mod tests {
    use super::{
        BuildVisibleFileTreeRowsOptions, FileTreeSearchHit, build_visible_file_tree_rows,
        compute_file_tree_search_hits, filter_file_tree_for_fff_hits, get_guide_ancestor_rows,
        get_sticky_ancestor_rows, sticky_ancestor_row_via_plural,
    };
    use crate::file_tree::types::AppFile;
    use std::collections::HashSet;

    fn dir(path: &str, name: &str, children: Vec<AppFile>) -> AppFile {
        AppFile {
            path: path.to_string(),
            name: name.to_string(),
            is_dir: true,
            children: Some(children),
            ..AppFile::default()
        }
    }

    fn file(path: &str, name: &str) -> AppFile {
        AppFile {
            path: path.to_string(),
            name: name.to_string(),
            is_dir: false,
            ..AppFile::default()
        }
    }

    // Mirrors the TS suite's fixture in visible-file-tree-rows.test.ts.
    fn tree() -> Vec<AppFile> {
        vec![dir(
            "/root",
            "root",
            vec![dir(
                "/root/src",
                "src",
                vec![dir(
                    "/root/src/features",
                    "features",
                    vec![dir(
                        "/root/src/features/file-explorer",
                        "file-explorer",
                        vec![file(
                            "/root/src/features/file-explorer/file-tree.tsx",
                            "file-tree.tsx",
                        )],
                    )],
                )],
            )],
        )]
    }

    fn paths(set: &[&str]) -> HashSet<String> {
        set.iter().map(|s| (*s).to_string()).collect()
    }

    // === buildVisibleFileTreeRows — ported from visible-file-tree-rows.test.ts ===

    #[test]
    fn shows_only_the_expanded_root_branch() {
        let rows = build_visible_file_tree_rows(
            &tree(),
            &paths(&["/root"]),
            BuildVisibleFileTreeRowsOptions::default(),
        );
        let got: Vec<&str> = rows.iter().map(|r| r.file.path.as_str()).collect();
        assert_eq!(got, vec!["/root", "/root/src"]);
        assert_eq!(rows.iter().map(|r| r.depth).collect::<Vec<_>>(), vec![0, 1]);
    }

    #[test]
    fn shows_third_level_rows_when_parent_folders_are_expanded() {
        let rows = build_visible_file_tree_rows(
            &tree(),
            &paths(&["/root", "/root/src", "/root/src/features"]),
            BuildVisibleFileTreeRowsOptions::default(),
        );
        let got: Vec<&str> = rows.iter().map(|r| r.file.path.as_str()).collect();
        assert_eq!(
            got,
            vec![
                "/root",
                "/root/src",
                "/root/src/features",
                "/root/src/features/file-explorer",
            ]
        );
        assert_eq!(
            rows.iter().map(|r| r.depth).collect::<Vec<_>>(),
            vec![0, 1, 2, 3]
        );
    }

    #[test]
    fn shows_deeper_descendants_once_every_ancestor_is_expanded() {
        let rows = build_visible_file_tree_rows(
            &tree(),
            &paths(&[
                "/root",
                "/root/src",
                "/root/src/features",
                "/root/src/features/file-explorer",
            ]),
            BuildVisibleFileTreeRowsOptions::default(),
        );
        let got: Vec<&str> = rows.iter().map(|r| r.file.path.as_str()).collect();
        assert_eq!(
            got,
            vec![
                "/root",
                "/root/src",
                "/root/src/features",
                "/root/src/features/file-explorer",
                "/root/src/features/file-explorer/file-tree.tsx",
            ]
        );
        assert_eq!(
            rows.iter().map(|r| r.depth).collect::<Vec<_>>(),
            vec![0, 1, 2, 3, 4]
        );
    }

    #[test]
    fn hides_nested_descendants_when_a_middle_folder_collapses() {
        // The ancestor-expansion branch this item's brief specifically asked
        // to be enumerated: /root/src/features is NOT in expanded_paths here,
        // so nothing below /root/src ever gets a row, regardless of what its
        // own descendants' expand-state might otherwise allow.
        let rows = build_visible_file_tree_rows(
            &tree(),
            &paths(&["/root", "/root/src"]),
            BuildVisibleFileTreeRowsOptions::default(),
        );
        let got: Vec<&str> = rows.iter().map(|r| r.file.path.as_str()).collect();
        assert_eq!(got, vec!["/root", "/root/src", "/root/src/features"]);
        assert_eq!(
            rows.iter().map(|r| r.depth).collect::<Vec<_>>(),
            vec![0, 1, 2]
        );
    }

    #[test]
    fn compacts_expanded_single_child_folder_chains() {
        let rows = build_visible_file_tree_rows(
            &tree(),
            &paths(&["/root", "/root/src", "/root/src/features"]),
            BuildVisibleFileTreeRowsOptions {
                compact_folders: true,
            },
        );
        let got: Vec<&str> = rows.iter().map(|r| r.file.path.as_str()).collect();
        assert_eq!(got, vec!["/root/src/features/file-explorer"]);
        assert_eq!(
            rows[0].display_name.as_deref(),
            Some("root/src/features/file-explorer")
        );
        assert_eq!(rows[0].depth, 0);
    }

    #[test]
    fn stops_compacting_at_the_collapsed_folder() {
        let rows = build_visible_file_tree_rows(
            &tree(),
            &paths(&["/root", "/root/src"]),
            BuildVisibleFileTreeRowsOptions {
                compact_folders: true,
            },
        );
        let got: Vec<&str> = rows.iter().map(|r| r.file.path.as_str()).collect();
        assert_eq!(got, vec!["/root/src/features"]);
        assert_eq!(rows[0].display_name.as_deref(), Some("root/src/features"));
        assert!(!rows[0].is_expanded);
    }

    // --- new: get_compact_folder_child's three None-returning guards,
    //     each exercised by a distinct scenario the ported TS suite's fixed
    //     fixture never produces ---

    #[test]
    fn compacting_does_not_step_past_a_folder_that_is_itself_mid_rename() {
        // get_compact_folder_child's first guard: an expanded folder that is
        // itself being renamed must interrupt the compact walk immediately,
        // even before looking at its children — this module's own doc says
        // so ("an in-progress rename mid-chain must interrupt the compact
        // walk so the user can see... that exact row").
        // No children: the guard this test targets short-circuits before
        // `get_compact_folder_child` ever looks at them, and omitting them
        // keeps the row count assertion below unambiguous (an expanded
        // directory WOULD otherwise recurse into real children regardless of
        // whether compaction itself succeeded).
        let renaming_root = AppFile {
            path: "/root".to_string(),
            name: "root".to_string(),
            is_dir: true,
            is_renaming: true,
            children: None,
            ..AppFile::default()
        };
        let rows = build_visible_file_tree_rows(
            &[renaming_root],
            &paths(&["/root"]),
            BuildVisibleFileTreeRowsOptions {
                compact_folders: true,
            },
        );
        assert_eq!(rows.len(), 1);
        assert_eq!(rows[0].file.path, "/root");
        assert!(rows[0].display_name.is_none());
    }

    #[test]
    fn compacting_stops_at_a_folder_with_more_than_one_child() {
        // get_compact_folder_child's second guard: exactly one child is
        // required to compact — two children (even both plain directories)
        // must stop the walk at the parent.
        let two_children = dir(
            "/root",
            "root",
            vec![dir("/root/a", "a", vec![]), dir("/root/b", "b", vec![])],
        );
        let rows = build_visible_file_tree_rows(
            &[two_children],
            &paths(&["/root"]),
            BuildVisibleFileTreeRowsOptions {
                compact_folders: true,
            },
        );
        assert_eq!(rows[0].file.path, "/root");
        assert!(rows[0].display_name.is_none());
    }

    #[test]
    fn compacting_stops_when_the_single_child_is_itself_mid_edit() {
        // get_compact_folder_child's third guard: the parent itself is a
        // clean single-child candidate, but the child being stepped into is
        // an in-progress inline-edit placeholder.
        let editing_child = AppFile {
            path: "/root".to_string(),
            name: "root".to_string(),
            is_dir: true,
            children: Some(vec![AppFile {
                path: "/root/new-folder".to_string(),
                name: String::new(),
                is_dir: true,
                is_editing: true,
                is_new_item: true,
                ..AppFile::default()
            }]),
            ..AppFile::default()
        };
        let rows = build_visible_file_tree_rows(
            &[editing_child],
            &paths(&["/root"]),
            BuildVisibleFileTreeRowsOptions {
                compact_folders: true,
            },
        );
        assert_eq!(rows[0].file.path, "/root");
        assert!(rows[0].display_name.is_none());
    }

    fn fully_expanded_rows() -> Vec<super::VisibleFileTreeRow> {
        build_visible_file_tree_rows(
            &tree(),
            &paths(&[
                "/root",
                "/root/src",
                "/root/src/features",
                "/root/src/features/file-explorer",
            ]),
            BuildVisibleFileTreeRowsOptions::default(),
        )
    }

    #[test]
    fn finds_the_nearest_sticky_ancestor_for_a_visible_descendant() {
        // Ported from the TS suite's `getStickyAncestorRow` (singular) test
        // — via the plural, per this module's doc on why the singular isn't
        // ported as its own function.
        let rows = fully_expanded_rows();
        assert_eq!(
            sticky_ancestor_row_via_plural(&rows, 4).map(|r| r.file.path),
            Some("/root/src/features/file-explorer".to_string())
        );
        assert_eq!(
            sticky_ancestor_row_via_plural(&rows, 2).map(|r| r.file.path),
            Some("/root/src".to_string())
        );
        assert_eq!(sticky_ancestor_row_via_plural(&rows, 0), None);
    }

    #[test]
    fn finds_the_full_sticky_ancestor_stack_for_a_visible_descendant() {
        let rows = fully_expanded_rows();
        let stack = get_sticky_ancestor_rows(&rows, 4);
        let got: Vec<&str> = stack.iter().map(|r| r.file.path.as_str()).collect();
        assert_eq!(
            got,
            vec![
                "/root",
                "/root/src",
                "/root/src/features",
                "/root/src/features/file-explorer",
            ]
        );
        assert!(get_sticky_ancestor_rows(&rows, 0).is_empty());
    }

    #[test]
    fn finds_guide_ancestors_for_each_visible_depth_level() {
        let rows = fully_expanded_rows();
        let guides = get_guide_ancestor_rows(&rows, 4);
        let got: Vec<Option<&str>> = guides
            .iter()
            .map(|r| r.as_ref().map(|row| row.file.path.as_str()))
            .collect();
        assert_eq!(
            got,
            vec![
                Some("/root"),
                Some("/root/src"),
                Some("/root/src/features"),
                Some("/root/src/features/file-explorer"),
            ]
        );
    }

    // --- new: not exercised by the ported TS suite directly, proving the
    //     branches this module's own doc enumerates ---

    #[test]
    fn get_sticky_ancestor_rows_is_empty_for_an_out_of_range_index() {
        let rows = fully_expanded_rows();
        assert!(get_sticky_ancestor_rows(&rows, 999).is_empty());
    }

    #[test]
    fn get_guide_ancestor_rows_is_empty_for_an_out_of_range_index() {
        let rows = fully_expanded_rows();
        assert!(get_guide_ancestor_rows(&rows, 999).is_empty());
    }

    #[test]
    fn get_guide_ancestor_rows_is_empty_for_a_root_level_row() {
        let rows = fully_expanded_rows();
        assert!(get_guide_ancestor_rows(&rows, 0).is_empty());
    }

    #[test]
    fn get_guide_ancestor_rows_leaves_a_gap_when_an_ancestor_depth_is_missing() {
        // A synthetic row list with a hole at depth 1 (no row of depth 1
        // exists between the depth-0 and depth-2 rows) proves the guide
        // function keeps `None` slots rather than compacting like the sticky
        // function does.
        let rows = vec![
            super::VisibleFileTreeRow {
                file: dir("/a", "a", vec![]),
                depth: 0,
                is_expanded: true,
                display_name: None,
            },
            super::VisibleFileTreeRow {
                file: file("/a/b/c", "c"),
                depth: 2,
                is_expanded: false,
                display_name: None,
            },
        ];
        let got = get_guide_ancestor_rows(&rows, 1);
        assert_eq!(got.len(), 2);
        assert_eq!(got[0].as_ref().map(|r| r.file.path.as_str()), Some("/a"));
        assert!(got[1].is_none());
    }

    // === computeFileTreeSearchHits — ported from file-tree-search-hits.test.ts ===

    fn search_tree() -> Vec<AppFile> {
        vec![
            file("README.md", "README.md"),
            file("Makefile", "Makefile"),
            dir(
                "src",
                "src",
                vec![
                    file("src/index.ts", "index.ts"),
                    file("src/readme-helper.ts", "readme-helper.ts"),
                ],
            ),
        ]
    }

    #[test]
    fn matches_a_file_by_case_insensitive_substring() {
        let hits = compute_file_tree_search_hits(&search_tree(), "readme");
        let got: Vec<&str> = hits.iter().map(|h| h.path.as_str()).collect();
        assert_eq!(got, vec!["README.md", "src/readme-helper.ts"]);
    }

    #[test]
    fn matches_a_directory_by_its_own_name_not_full_path() {
        let hits = compute_file_tree_search_hits(&search_tree(), "src");
        let got: Vec<&str> = hits.iter().map(|h| h.path.as_str()).collect();
        assert_eq!(got, vec!["src"]);
    }

    #[test]
    fn returns_no_hits_for_an_empty_or_whitespace_query() {
        assert!(compute_file_tree_search_hits(&search_tree(), "").is_empty());
        assert!(compute_file_tree_search_hits(&search_tree(), "   ").is_empty());
    }

    #[test]
    fn returns_no_hits_when_nothing_matches() {
        assert!(compute_file_tree_search_hits(&search_tree(), "zzz-nope").is_empty());
    }

    #[test]
    fn skips_in_progress_inline_edit_placeholder_nodes() {
        let with_placeholder = vec![
            AppFile {
                path: "src/".to_string(),
                name: String::new(),
                is_dir: false,
                is_new_item: true,
                is_editing: true,
                ..AppFile::default()
            },
            file("real.ts", "real.ts"),
        ];
        let hits = compute_file_tree_search_hits(&with_placeholder, "r");
        let got: Vec<&str> = hits.iter().map(|h| h.path.as_str()).collect();
        assert_eq!(got, vec!["real.ts"]);
    }

    // === filterFileTreeForFffHits — ported from visible-file-tree-rows.test.ts ===

    #[test]
    fn keeps_matching_files_with_their_ancestors_expanded() {
        let result = filter_file_tree_for_fff_hits(
            &tree(),
            &[FileTreeSearchHit {
                path: "/root/src/features/file-explorer/file-tree.tsx".to_string(),
            }],
        );
        let rows = build_visible_file_tree_rows(
            &result.files,
            &result.expanded_paths,
            BuildVisibleFileTreeRowsOptions::default(),
        );
        let got: Vec<&str> = rows.iter().map(|r| r.file.path.as_str()).collect();
        assert_eq!(
            got,
            vec![
                "/root",
                "/root/src",
                "/root/src/features",
                "/root/src/features/file-explorer",
                "/root/src/features/file-explorer/file-tree.tsx",
            ]
        );
        assert_eq!(
            result.matched_paths,
            paths(&["/root/src/features/file-explorer/file-tree.tsx"])
        );
        assert_eq!(
            result.ordered_matched_paths,
            vec!["/root/src/features/file-explorer/file-tree.tsx".to_string()]
        );
        assert_eq!(result.match_count, 1);
    }

    #[test]
    fn keeps_a_matched_folder_without_expanding_unmatched_descendants() {
        let result = filter_file_tree_for_fff_hits(
            &tree(),
            &[FileTreeSearchHit {
                path: "/root/src/features".to_string(),
            }],
        );
        let rows = build_visible_file_tree_rows(
            &result.files,
            &result.expanded_paths,
            BuildVisibleFileTreeRowsOptions::default(),
        );
        let got: Vec<&str> = rows.iter().map(|r| r.file.path.as_str()).collect();
        assert_eq!(got, vec!["/root", "/root/src", "/root/src/features"]);
        assert_eq!(result.matched_paths, paths(&["/root/src/features"]));
    }

    #[test]
    fn returns_an_empty_tree_for_empty_fff_results() {
        let result = filter_file_tree_for_fff_hits(&tree(), &[]);
        assert!(result.files.is_empty());
        assert_eq!(result.match_count, 0);
        assert!(result.expanded_paths.is_empty());
    }

    // --- new: not exercised by the ported TS suite ---

    #[test]
    fn normalize_search_path_leaves_the_filesystem_root_untouched() {
        // The one path normalize_search_path special-cases: every other
        // input has ITS trailing slashes stripped, but "/" alone would
        // otherwise strip down to "", which is not the same path.
        assert_eq!(super::normalize_search_path("/"), "/");
        assert_eq!(super::normalize_search_path("/root/"), "/root");
    }

    #[test]
    fn a_hit_whose_path_never_appears_in_the_tree_contributes_nothing() {
        let result = filter_file_tree_for_fff_hits(
            &tree(),
            &[FileTreeSearchHit {
                path: "/root/does/not/exist".to_string(),
            }],
        );
        assert!(result.files.is_empty());
        assert_eq!(result.match_count, 0);
    }

    #[test]
    fn a_new_item_placeholder_at_the_workspace_root_never_counts_as_expanded() {
        // Regression the TS source's own comment documents: an isNewItem
        // placeholder can have path == '' (the root), which must never be
        // treated as an expanded directory even if '' is in expanded_paths.
        let placeholder_at_root = vec![AppFile {
            path: String::new(),
            name: String::new(),
            is_dir: true,
            is_new_item: true,
            children: Some(vec![file("should-not-appear.ts", "should-not-appear.ts")]),
            ..AppFile::default()
        }];
        let rows = build_visible_file_tree_rows(
            &placeholder_at_root,
            &paths(&[""]),
            BuildVisibleFileTreeRowsOptions::default(),
        );
        assert_eq!(rows.len(), 1);
        assert!(!rows[0].is_expanded);
    }
}
