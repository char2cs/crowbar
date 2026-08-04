# `crowbar-core::file_tree` — the file-tree-model area (P3.75)

Five files' worth of exports from `native/mapping/tier-a-denominator.md` §8's
export-level "Deliverable 3 — scope recommendation" for §5 ("File-tree
model") ported into `native/crates/crowbar-core/src/file_tree/`, the fifth
Tier A area to land after workspace scoping (P3.53), the git model (P3.67),
keymap resolution (P3.70) and settings schema (P3.72). Scope was taken from
§8's export-level audit, per this item's own instruction — **not** the
survey's file-level table or prose, both of which §8 itself documents as
having previously described dead/test-only exports as though they were
individually importable. Each module carries its own doc comment citing the
TS source and any surprising behaviour; this file is the cross-module index.

## 1. What each module models

| module | ported from | TS tests? |
|---|---|---|
| [`types`] | `types/app.ts` (`AppFile`/`FileEntry`/`ContextMenuState`) | none — 3 authored |
| [`visible_rows`] | `lib/visible-file-tree-rows.ts` (5 of 6 exports) | `visible-file-tree-rows.test.ts` (12 cases) + `file-tree-search-hits.test.ts` (5 cases) = 17 ported, plus 10 authored (27 total) |
| [`git_status`] | `lib/file-tree-git-status.ts` (all 6 exports) | `file-tree-git-status.test.ts` (9 cases), ported, plus 12 authored (21 total) |
| [`density`] | `lib/file-tree-density.ts` (4 of 6 exports, plus `rowHeight` only) | 2 cases ported from `settings-normalization.test.ts` (the pass-through cases it exercises), plus 5 authored (7 total) |
| [`tree_utils`] | `utils/file-explorer-tree-utils.ts` (1 of 5 exports) | `file-explorer-tree-utils.test.ts`'s `getExplorerTargetPath` describe (2 cases), ported, plus 3 authored (5 total) |
| [`file_name`] | `../../file-system/controllers/file-utils.ts` (2 names, 1 function) | none — no TS test file exists for this source either; 4 authored |

- **`types`** — [`types::AppFile`] (aliased [`types::FileEntry`]), trimmed
  from the TS interface's 13 fields to the 7 this crate's ported functions
  actually read or write (`path`, `name`, `is_dir`, `is_renaming`,
  `is_editing`, `is_new_item`, `children`). See [`types`]'s module doc for
  which 6 fields were dropped and why (`isDirectory` is a dead alias of
  `isDir` — nothing in `web/src` reads it; `isFile`/`isSymlink`/
  `symlinkTarget`/`ignored`/`gitStatus` are read only by files this item
  doesn't port). [`types::ContextMenuState`] is also ported — the brief
  names it explicitly alongside `AppFile`/`FileEntry` — even though none of
  this item's five in-scope functions read or construct one; see its own
  doc for that caveat stated plainly rather than silently dropped.
- **`visible_rows`** — the substantive algorithm this whole area is named
  for: [`visible_rows::build_visible_file_tree_rows`] (nested tree + expand
  state → flat visible-row list, with compact single-child-folder
  collapsing), [`visible_rows::compute_file_tree_search_hits`] /
  [`visible_rows::filter_file_tree_for_fff_hits`] (name-substring search
  with ancestor auto-expansion), [`visible_rows::get_sticky_ancestor_rows`] /
  [`visible_rows::get_guide_ancestor_rows`] (breadcrumb/indent-guide
  support). See §2 below for the branch enumeration this item's brief
  required before writing any Rust.
- **`git_status`** — git-status → per-row decoration, with directory-level
  status propagated from the highest-priority changed descendant. See §3
  below for the `colorClassName` split this item's brief specifically
  required.
- **`density`** — the row-density enum, its normalizer, and (`rowHeight`
  only) its per-density config table. See §4 below: this module is also the
  settings/file-tree reconciliation `crate::settings::types::FileTreeDensity`
  flagged itself as needing, back when it was ported first.
- **`tree_utils`** — [`tree_utils::get_explorer_target_path`], the file's
  one live export. See its own module doc for why the other four
  (`filterHiddenFiles`, `addNewItemToTree`, `removeEditingItemsFromTree`,
  the exported `getAncestorDirectoryPaths`) are not ported, and for
  [`tree_utils::ExplorerTargetBuffer`] — a narrow local stand-in for
  `PaneContent` (a ten-variant union this item does not port in full; see
  its doc for the reconciliation this leaves for whichever item ports panes
  next).
- **`file_name`** — [`file_name::get_file_name`], ported once under one
  name. `getFileName`/`getFilenameFromPath` are the same function in the TS
  source (`export const getFilenameFromPath = getFileName`, a literal alias,
  not a second implementation) — **this port kept the base name,
  `get_file_name`**, since that is the one real implementation; the live
  call site (`editor-status-actions.tsx`) happens to import the alias
  spelling, but both names always resolved to the identical function, so
  there is nothing a second Rust name would add.

## 2. `buildVisibleFileTreeRows`'s real branches, enumerated before writing any Rust

Per this item's own instruction to read the TypeScript branch-by-branch
before porting:

- **A collapsed ancestor hides everything below it**, regardless of the
  descendant's own expand state. The recursion only descends into a
  directory's `children` when *that* directory's own path is in
  `expanded_paths` — a descendant three levels deep behind one collapsed
  ancestor never gets a row even if its own path is separately marked
  expanded. Tested: `hides_nested_descendants_when_a_middle_folder_collapses`
  (ported from the TS suite) and a dedicated ancestor-expansion mutation
  (§6, mutation 1).
- **An empty (or whitespace-only) search query is a no-op**, not "nothing
  changed since the last query" — `compute_file_tree_search_hits` returns
  `[]` immediately without walking the tree at all, matching the TS
  source's own early `if (!q) return []`.
- **A match's ancestors are re-added even when the ancestors themselves
  don't match the query** — `filter_file_tree_for_fff_hits`'s recursion
  keeps a non-matching directory in the output whenever at least one
  (possibly deeply nested) descendant matched. An ancestor that is EXCLUDED
  (a sibling subtree with no match anywhere in it) is dropped from the
  output tree entirely, not merely left uninspected.
- **A matched directory does not auto-expand its own non-matching
  children** — `expanded_paths` only gains a directory's path when its
  `matching_children` (the recursion result) is non-empty; a matched
  *folder itself*, with no matching descendant, is kept in the output tree
  (so it still renders) but stays collapsed. Tested:
  `keeps_a_matched_folder_without_expanding_unmatched_descendants` and a
  dedicated mutation (§6, mutation 2).
- **An empty hit list returns a genuinely empty result**, not "the whole
  tree, nothing highlighted" — `filterFileTreeForFffHits(files, [])`
  short-circuits before touching `files` at all.
- **A brand-new inline-edit placeholder never counts as expanded**, even if
  its path (which can be `''`, the workspace root) happens to be in
  `expanded_paths` — otherwise a new-item placeholder at the root would load
  the whole workspace root as its own children and duplicate the tree.
  Tested directly (`a_new_item_placeholder_at_the_workspace_root_never_counts_as_expanded`).
- **Compacting a single-child folder chain stops at three distinct guards**,
  each proven by its own test (§6's mutation set and the coverage pass, §7):
  the folder being compacted-*from* must not itself be mid-rename/mid-edit,
  it must have exactly one child, and that child must not itself be
  mid-rename/mid-edit — an in-progress rename anywhere in the chain must
  interrupt compaction so the user can see (and finish editing) that exact
  row.

## 3. The `colorClassName` split

`file-tree-git-status.ts`'s `getFileTreeGitStatusDecoration` returns
`{ colorClassName, label, statusLetter }` — a hardcoded Tailwind class
string bundled with the genuine classification. Per this item's brief, the
two are split at the type level, not just re-typed in place:

- [`git_status::FileTreeGitStatusDecoration`] is a 6-variant enum
  (`Modified`, `ModifiedStaged`, `Added`, `Deleted`, `Untracked`, `Renamed`)
  carrying the classification — `.status_letter()` and `.label()` — with
  **no colour field at all**. TS's three fields always co-vary (there is no
  `'modified'`/`'A'`/`'Deleted'` combination the original `switch` can
  produce), so sealing this as an enum instead of a struct-of-three-strings
  makes an invalid combination unrepresentable, not merely untested.
- [`git_status::GitStatusColor`] is a second, wholly separate enum with the
  same six variant names but zero string content — reachable only via
  `.color()`. `crowbar-core` must never depend on `gpui` (§4.3 rule 1) and
  has no business owning a Tailwind class string regardless; `crowbar-ui`
  (once it exists) is the intended resolver, matching a `crowbar-ui::Color`
  seal the way `crate::color`'s own module doc establishes the
  arithmetic/painting boundary for CSS `color-mix`.

A caller that only wants the classification never touches
`GitStatusColor`, and nothing in this crate ever converts a
`GitStatusColor` into a paintable value — that absence is the point.

## 4. The `FileTreeDensity` reconciliation

`crate::settings::types::FileTreeDensity`'s own module doc, written during
the settings-schema item (P3.72) before this item existed, flagged itself
explicitly: *"a narrow, deliberate duplicate of the 3-variant TS type — when
file-tree model is ported, this type should be deleted in favour of that
module's, and every reference here repointed."* This item is that
reconciliation:

- [`density::FileTreeDensity`] and [`density::normalize_file_tree_density`]
  are now the one definition in the crate.
- `crate::settings::types` no longer declares its own copy — it re-exports
  both names (`pub use crate::file_tree::density::{FileTreeDensity,
  normalize_file_tree_density};`), so every existing caller
  (`crate::settings::defaults::default_settings`, the `Settings` struct's
  `file_tree_density` field, and `crate::settings`'s own top-level
  re-exports) compiles unchanged — only the *definition's location* moved,
  not its name or shape.
- The four tests that used to live in `settings/types.rs` for this
  normalizer moved to `file_tree/density.rs`, the module that now owns the
  definition, rather than being duplicated in both places.

## 5. What was deliberately not ported, and why

All CONDITIONAL, DEAD, or out-of-scope per `tier-a-denominator.md` §8's
export-level audit and this item's own brief:

- **`visible-file-tree-rows.ts`'s `getStickyAncestorRow` (singular)** — §8's
  audit found zero non-test callers anywhere in `web/src`; the plural
  [`visible_rows::get_sticky_ancestor_rows`] is what every real call site
  (`file-explorer-tree.tsx`, 7 call sites) actually uses. The singular's own
  TS body is `getStickyAncestorRows(rows, firstVisibleIndex)[length - 1] ??
  null` — calling the plural and reading `.last()` reproduces it exactly;
  see [`visible_rows`]'s module doc for the one-line equivalence and its
  test-only helper proving it.
- **`file-explorer-tree-utils.ts`'s other four exports**
  (`filterHiddenFiles`, `addNewItemToTree`, `removeEditingItemsFromTree`,
  the exported `getAncestorDirectoryPaths`) — `filterHiddenFiles` and
  `removeEditingItemsFromTree` have zero non-test references anywhere,
  including their own file (their only self-file hit is pure recursion, not
  a caller); `addNewItemToTree` and `getAncestorDirectoryPaths` are each
  independently redeclared locally in the files that actually need that
  behaviour (`use-file-explorer-inline-editing.ts`,
  `file-tree-gitignore.ts`), so the exported originals are never called.
  `getAncestorDirectoryPaths` even has a dedicated TS test exercising the
  unreachable exported copy — not ported here for the same reason
  `crate::git`'s module doc gives for a different dead type: a declared,
  never-constructed/never-called shape is not live code.
- **`file-tree-gitignore.ts`** — explicitly out of this item's scope per the
  brief: it needs a Rust `ignore`-crate dependency decision
  (`tier-a-denominator.md` §8 recommends `ignore::gitignore::Gitignore`, one
  instance per directory, with the ancestor-first cascade reimplemented by
  hand — no crate equivalent for that part) that this item does not make.
- **The 7 hook/store files** (`file-explorer-tree-store.ts`,
  `file-explorer-clipboard-store.ts`, and 5 `use-file-explorer-*` hooks,
  1,478 lines) — Phase-4 glue; GPUI's own action/keybinding and `Entity<T>`
  machinery replaces this wiring shape wholesale, the same convention
  `crate::keymap`'s module doc already establishes for the analogous
  keyboard-hook layer.
- **`env-template.ts`, `file-upload.ts`** — CONDITIONAL, gated behind a
  specific right-click context-menu action; `file-upload.ts` is also
  `FileReader`/`<input type=file>`-bound platform code, not core logic.
- **`file-tree-api.ts`** — transport (`fetchFileTree`, `createFileNode`,
  ...); `crowbar-client` territory per this item's brief, not
  `crowbar-core`.
- **`findFileInTree`** (`file-system/controllers/file-tree-utils.ts`) — a
  separate concern per this item's brief; left unported.
- **`file-tree-density.ts`'s `FILE_TREE_DENSITY_CONFIG.rowClassName` and
  `FILE_TREE_DENSITY_OPTIONS`** — presentation (a Tailwind class string,
  and Settings-tab dropdown copy respectively).

## 6. Mutation testing

Five mutations were made (the brief asked for at least four, including one
on the ancestor-expansion path); each was watched to fail for real, then
reverted with `cp` (fresh mtime — see the note at the end of this section)
rather than `mv`-from-backup, and the crate's tests were re-run after every
revert to confirm the revert actually recompiled clean. Real output, pasted
verbatim:

1. **Ancestor-expansion path — `walk_visible_rows`'s `is_expanded`
   computation dropped its `expanded_paths.contains(...)` check**
   (`visible_rows.rs`), so every directory rendered as if always expanded
   regardless of the caller's actual expand-state set. 6 tests failed:

   ```
   thread '...' panicked at crates/crowbar-core/src/file_tree/visible_rows.rs:544:9:
   assertion `left == right` failed
     left: ["/root", "/root/src", "/root/src/features", "/root/src/features/file-explorer", "/root/src/features/file-explorer/file-tree.tsx"]
    right: ["/root", "/root/src", "/root/src/features"]
   test result: FAILED. 15 passed; 6 failed; 0 ignored; 0 measured; 260 filtered out
   ```

   (`hides_nested_descendants_when_a_middle_folder_collapses`,
   `shows_only_the_expanded_root_branch`,
   `shows_third_level_rows_when_parent_folders_are_expanded`,
   `compacts_expanded_single_child_folder_chains`,
   `stops_compacting_at_the_collapsed_folder`,
   `keeps_a_matched_folder_without_expanding_unmatched_descendants`.)
   Reverted.

2. **The "matched folder doesn't auto-expand" guard —
   `filter_file_tree_for_fff_hits` dropped the `!matching_children.is_empty()`
   half of its `if item.is_dir && !matching_children.is_empty()` condition**
   (`visible_rows.rs`), so every matched directory was marked expanded
   regardless of whether anything inside it actually matched.

   ```
   thread '...' panicked at crates/crowbar-core/src/file_tree/visible_rows.rs:797:9:
   assertion `left == right` failed
     left: ["/root", "/root/src", "/root/src/features", "/root/src/features/file-explorer"]
    right: ["/root", "/root/src", "/root/src/features"]
   test result: FAILED. 20 passed; 1 failed; 0 ignored; 0 measured; 260 filtered out
   ```

   (`keeps_a_matched_folder_without_expanding_unmatched_descendants`.)
   Reverted.

3. **Priority table — `git_status_priority`'s `"deleted" => 50` changed to
   `"deleted" => 5`** (`git_status.rs`), so a deleted file no longer
   outranked modified/renamed/untracked descendants when propagating
   directory-level status.

   ```
   thread '...' panicked at crates/crowbar-core/src/file_tree/git_status.rs:536:9:
   assertion `left == right` failed
     left: Some(Modified)
    right: Some(Deleted)
   test result: FAILED. 13 passed; 1 failed; 0 ignored; 0 measured; 267 filtered out
   ```

   (`uses_the_highest_priority_descendant_status_for_directories`.)
   Reverted.

4. **`get_relative_path`'s offset arithmetic — dropped the `+ 1` for a
   root path that doesn't already end in `/`** (`git_status.rs`), so every
   relative path kept a leading slash and no longer matched the lookup's
   keys.

   ```
   thread '...' panicked at crates/crowbar-core/src/file_tree/git_status.rs:532:9:
   assertion `left == right` failed
     left: None
    right: Some(Deleted)
   thread '...' panicked at crates/crowbar-core/src/file_tree/git_status.rs:491:9:
   assertion `left == right` failed
     left: None
    right: Some(Modified)
   test result: FAILED. 12 passed; 2 failed; 0 ignored; 0 measured; 267 filtered out
   ```

   (`uses_the_highest_priority_descendant_status_for_directories`,
   `keeps_exact_file_status_and_inherited_directory_status_separate`.)
   Reverted.

5. **`density.rs`'s `row_height` table — `Compact => 20` changed to
   `Compact => 99`.**

   ```
   thread '...' panicked at crates/crowbar-core/src/file_tree/density.rs:141:9:
   assertion `left == right` failed
     left: 99
    right: 20
   test result: FAILED. 6 passed; 1 failed; 0 ignored; 0 measured; 274 filtered out
   ```

   (`row_height_matches_the_ts_config_table`.) Reverted.

**On the mtime trap** (per this item's own warning): every revert above used
`cp` from a pre-mutation backup copy back onto the mutated file, followed by
`touch`, then a fresh `cargo test` run — never `mv`-from-backup, which would
carry the backup's older mtime and risk `cargo` serving a stale compiled
test binary that silently "still passes" a reverted mutation. Each revert
was diffed against the pre-mutation copy (`diff` reported identical) and
its corresponding test suite re-run green before moving to the next
mutation, confirmed by the passed-count changing (e.g. mutation 1's 21/21
green after revert, not a suspiciously-unchanged 15/6).

## 7. Coverage

`cargo llvm-cov -p crowbar-core`: **100.00% line coverage over 3,683 lines**
(up from **2,531 lines, 100.00%** before this item — the P3.72 settings
baseline), a net growth of **1,152 lines**. The six new `file_tree/*.rs`
files themselves total **1,173** covered lines — `density.rs` 48,
`file_name.rs` 16, `git_status.rs` 438, `tree_utils.rs` 48, `types.rs` 29,
`visible_rows.rs` 594 (executable lines as `cargo llvm-cov` counts them —
smaller than each file's `wc -l`, which also counts doc comments and blank
lines) — offset by a **21-line reduction** in `settings/types.rs`'s own
covered-line count from removing its now-redundant
`FileTreeDensity`/`normalize_file_tree_density` declaration and its four
tests (§4): `1,173 − 21 = 1,152`, the crate's actual net growth.

The first coverage run was **not** 100% (99.13% line coverage, 20 missed
lines across `git_status.rs` and `visible_rows.rs`). Every miss was a real,
uncovered branch, closed with a test proving the actual behaviour rather
than left as an unexplained percentage:

- `FileTreeGitStatusDecoration::label()`'s `Added`/`Deleted`/`Untracked`/
  `Renamed` arms — the original test only checked `.label()` for
  `Modified`/`ModifiedStaged`, leaving the other four label strings
  unverified even though `.status_letter()` checked all six.
- `create_file_tree_git_status_lookup`'s `continue` guard for an
  unrecognised status — no test previously proved a file with an unknown
  status is excluded from the lookup entirely, not merely undecorated.
- `get_git_status_priority`'s staged-modification `+1` tie-breaker — no
  test previously constructed the actual tie (an unstaged and a staged
  `modified` file in the same directory) the bonus exists to break.
- The private `getRelativePath` port's Windows-style-path branches
  (`is_drive_root`, the two `to_lowercase()` case-insensitive-compare arms,
  the `root_for_compare.ends_with('/')` arm) — none of this item's ported
  TS test cases use a drive-letter or backslash path, so these branches had
  no test at all; four new tests exercise them directly, read from
  `path-helpers.ts`'s own source rather than a TS test file (that file is
  out of this item's scope).
- Three of `get_compact_folder_child`'s guards (the folder-being-compacted
  mid-rename, more than one child, the single child itself mid-edit) — the
  ported TS fixture's fixed tree only ever exercises the *successful*
  compaction path plus the "stop because the loop's own `while` condition
  went false" path; none of its cases make `get_compact_folder_child`
  itself return `None`. Three new tests construct each guard directly (§2's
  branch enumeration already named this as a real gap).
- `get_guide_ancestor_rows`'s out-of-range-index and depth-0 early returns,
  and `get_sticky_ancestor_rows`'s sibling out-of-range case for the guide
  function specifically (the sticky function's own case was already
  covered) — not exercised by the ported TS suite, which never calls either
  function with an invalid index.
- `normalize_search_path`'s `"/"`-is-special-cased branch — no ported test
  searches for a hit whose path is the bare filesystem root.
- `git_status_priority`'s `_ => 0` fallback arm — unreachable via any public
  entry point (the one caller already filters to the five known statuses
  before calling in, mirroring the TS `Record`'s closed-union guarantee),
  exercised directly against the private function rather than left
  untested — the same treatment `file_name::get_file_name`'s own
  `unwrap_or(path)` fallback gets (documented as unreachable, kept for
  fidelity to the TS `?? path`, not chased into an artificial test since
  `str::rsplit` genuinely cannot return zero items).

After closing all of the above, line coverage reached 100.00%; two files
(`git_status.rs` 99.69% region / 100.00% line, `visible_rows.rs` 99.65%
region / 100.00% line) still show a small region-coverage gap with zero
missed lines — the same shape this crate's own `keymap/chord.rs` (96.62%
region / 100.00% line) and `workspace/scope.rs` (99.72% region / 100.00%
line) already have, per `core-settings.md` §8's own precedent: sub-line
match-arm/wildcard bookkeeping, not uncovered behaviour.

294 tests in `crowbar-core`'s lib target after this item (**231 before** —
the P3.72 settings baseline — **231 - 4 removed from `settings/types.rs`'s
now-deleted duplicate + 67 new in `file_tree::*` = 294**, i.e. a net +63:
67 new tests added, 4 old ones relocated rather than duplicated). Of the 67
new, 30 have a ported TS counterpart: 12
(`visible-file-tree-rows.test.ts`'s `buildVisibleFileTreeRows` describe, 9
cases, + its `filterFileTreeForFffHits` describe, 3 cases) + 5
(`file-tree-search-hits.test.ts`) + 9 (`file-tree-git-status.test.ts`'s
three describes, 2 + 4 + 3 cases) + 2 (`getExplorerTargetPath`, from
`file-explorer-tree-utils.test.ts`) + 2 (`density.rs`'s
`accepts_a_known_density_unchanged` and
`falls_back_to_default_for_an_unknown_density`, relocated from
`settings/types.rs`, both originally ported from
`settings-normalization.test.ts`'s one pass-through case). The remaining 37
are authored: branch-enumeration cases per §2, the coverage-closing cases
per this section, `density.rs`'s other 5 cases, and all of `types.rs`'s (3)
and `file_name.rs`'s (4), since `types/app.ts` and `file-utils.ts` have no
TS test file at all.

Full-workspace gates, run in the foreground: `cargo clippy --workspace
--all-targets -- -D warnings` clean; `cargo test --workspace` — **2,394
passed, 0 failed** (up from the 2,331-test trunk baseline plus this item's
63 net new `crowbar-core` tests); `bash native/scripts/check-invariants.sh`
— 7 of 7 invariants pass, including `cargo fmt --check` (run after this
item's files were formatted with `cargo fmt -p crowbar-core`).

## 8. What this pass found, restated plainly

Re-reading everything above for claims that outran their evidence, per this
item's own closing instruction:

- **`git_status.rs`'s `.label()` method had 4 of 6 arms untested despite
  looking fully covered at a glance** — the original test asserted every
  `.status_letter()` arm but only two of six `.label()` arms, an asymmetry
  the first `cargo llvm-cov` run caught and this document's §7 records
  rather than silently patching over.
- **The `FileTreeDensity` reconciliation (§4) is real, not aspirational** —
  `crate::settings::types.rs` no longer contains a second enum declaration;
  it is a `pub use` of this module's type, verified by `cargo build`
  succeeding with `Settings.file_tree_density`'s type unchanged and by
  `settings/defaults.rs` needing zero edits.
- **The singular `getStickyAncestorRow` was not ported**, per the brief's
  explicit instruction — verified again independently here rather than
  taken on the brief's word: `grep -rn "getStickyAncestorRow[^s]"
  web/src/features/file-explorer` finds only the TS declaration itself, no
  production call site anywhere under `features/`; a separate search of
  `web/src/__tests__` finds exactly one hit, its own dedicated test file.
- **`ContextMenuState` genuinely has zero producer/consumer among this
  item's five in-scope functions** — ported anyway because the brief names
  it explicitly, and that caveat is stated in its own module doc rather
  than left for a future reader to rediscover by grepping.
- No file-level/export-level miscount was found in this pass (unlike the
  P3.70/P3.67 items' own findings) — the six files/exports named in the
  brief matched §8's own export-level scope recommendation exactly, checked
  against §8's per-export tables directly before writing any Rust, not
  taken on the brief's summary alone.

[`types`]: ../crates/crowbar-core/src/file_tree/types.rs
[`types::AppFile`]: ../crates/crowbar-core/src/file_tree/types.rs
[`types::FileEntry`]: ../crates/crowbar-core/src/file_tree/types.rs
[`types::ContextMenuState`]: ../crates/crowbar-core/src/file_tree/types.rs
[`visible_rows`]: ../crates/crowbar-core/src/file_tree/visible_rows.rs
[`visible_rows::build_visible_file_tree_rows`]: ../crates/crowbar-core/src/file_tree/visible_rows.rs
[`visible_rows::compute_file_tree_search_hits`]: ../crates/crowbar-core/src/file_tree/visible_rows.rs
[`visible_rows::filter_file_tree_for_fff_hits`]: ../crates/crowbar-core/src/file_tree/visible_rows.rs
[`visible_rows::get_sticky_ancestor_rows`]: ../crates/crowbar-core/src/file_tree/visible_rows.rs
[`visible_rows::get_guide_ancestor_rows`]: ../crates/crowbar-core/src/file_tree/visible_rows.rs
[`git_status`]: ../crates/crowbar-core/src/file_tree/git_status.rs
[`git_status::FileTreeGitStatusDecoration`]: ../crates/crowbar-core/src/file_tree/git_status.rs
[`git_status::GitStatusColor`]: ../crates/crowbar-core/src/file_tree/git_status.rs
[`density`]: ../crates/crowbar-core/src/file_tree/density.rs
[`density::FileTreeDensity`]: ../crates/crowbar-core/src/file_tree/density.rs
[`density::normalize_file_tree_density`]: ../crates/crowbar-core/src/file_tree/density.rs
[`tree_utils`]: ../crates/crowbar-core/src/file_tree/tree_utils.rs
[`tree_utils::get_explorer_target_path`]: ../crates/crowbar-core/src/file_tree/tree_utils.rs
[`tree_utils::ExplorerTargetBuffer`]: ../crates/crowbar-core/src/file_tree/tree_utils.rs
[`file_name::get_file_name`]: ../crates/crowbar-core/src/file_tree/file_name.rs
