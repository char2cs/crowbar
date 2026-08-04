# `crowbar-core::git` — the git-model area (P3.67)

Six files from `native/mapping/tier-a-denominator.md` §"Git model" ported into
`native/crates/crowbar-core/src/git/`, the second Tier A area to land after
workspace scoping (P3.53). Each module carries its own doc comment citing the
TS source and any surprising behaviour; this file is the cross-module index
the task asked for: what each module models, and what was deliberately not
ported, with the reason.

`wc -l` on each Rust file (doc comments, tests and all) against the TS
source's line count from `tier-a-denominator.md`:

| module | ported from | TS lines | Rust `wc -l` |
|---|---|---|---|
| `types.rs` | fields of `types/git-types.ts`'s `GitDiff` this item uses | (subset of 78) | 70 |
| `git_status_to_changed_files.rs` | `utils/git-status-to-changed-files.ts` | 45 | 147 |
| `build_git_folder_tree.rs` | `utils/build-git-folder-tree.ts` | 57 | 343 |
| `review_file_summary_to_git_diff.rs` | `utils/review-file-summary-to-git-diff.ts` | 41 | 161 |
| `normalize_diff.rs` | `utils/normalize-diff.ts` | 38 | 177 |
| `diff_buffer_path.rs` | `utils/diff-buffer-path.ts` | 24 | 317 |
| `branch_action.rs` | `lib/branch-action.ts` | 49 | 231 |
| `mod.rs` | (new — index + type-reuse rationale) | — | 80 |

The Rust files run larger than their TS sources throughout — doc comments
citing the source and explaining every divergence, ported-vs-new test
provenance comments, and (for three modules) real mutation-testing
transcripts pasted verbatim, none of which the TS originals carry. The
`cargo llvm-cov`-measured *executable* line counts (§6) are the ones that
matter for the coverage gate, and are smaller than `wc -l` for the same
reason.

## 1. What each module models

- **`types::GitDiff`** — the unified sidebar-tree projection type both
  `git_status_to_changed_files` and `review_file_summary_to_git_diff`
  produce. Hand-rolled rather than reusing `crowbar_proto::domain_git::FileDiff`
  because `FileDiff.additions`/`.deletions`/`.uncommitted` are non-optional,
  and one of the two producers (`git_status_to_changed_files`) genuinely has
  no count data to put there — see the module doc for the full argument.
- **`git_status_to_changed_files`** — projects the cheap working-tree status
  (`GitFileDTO[]`, no per-file counts) into `GitDiff[]`, deduping a path
  listed twice (staged + unstaged) by first occurrence.
- **`build_git_folder_tree`** — flat `GitFileDTO[]` → nested folder tree
  (`GitFolderNode`), plus the three small utilities the TS source ships
  alongside it (`sort_folders_by_name`, `sort_files_by_path`,
  `collect_node_files`).
- **`review_file_summary_to_git_diff`** — projects the files-only branch-review
  summary (`ReviewFileSummary[]`, which DOES carry counts, with a `-1` binary
  sentinel) into the same `GitDiff[]` shape.
- **`normalize_diff`** — does **not** port two pass-through functions. See §2.
- **`diff_buffer_path`** — parses the synthetic `diff://…` buffer-path scheme
  (staged/unstaged/commit/stash) back to the real file path it addresses,
  including a hand-derived `.diff`-suffix-stripping edge case the upstream
  regex encodes but no existing test exercises.
- **`branch_action`** — `resolveBranchAction`, the precedence-ordered pure
  decision function (`commit > resolve > pull-request > merge > sync-only`).

## 2. `normalize_diff.ts` — ported as an invariant, not two functions

`normalizeGitDiff`/`normalizeMultiFileDiff` defended TS's `GitDiff.lines`
against arriving `null` — a stale-persisted-shape bug (an opened diff tab's
payload is embedded in a `localStorage`-backed cache; a tab opened before a
daemon-side `MarshalJSON` fix keeps reappearing with the old shape on every
relaunch, with no refetch to correct it).

This does not port as two functions in Rust, for two independent reasons
argued in full in `normalize_diff.rs`'s module doc:

1. `Vec<T>` cannot be null in memory — there is no runtime state matching the
   bug's shape for a function to observe and repair, once a `FileDiff`/
   `GitDiff` value exists at all.
2. `crowbar_proto::domain_git::FileDiff.lines` (and `MultiFileDiff.files`)
   already deserialize through `null_default::null_to_default`, which maps a
   wire `null` onto `vec![]`. Unlike the TS history, there was never a version
   of this port's `FileDiff` without that guard, so there is no "opened
   before the fix" era of persisted payloads to defend against retroactively.

A function that can only ever return its input unchanged, backed by a test
that can only assert that unconditional fact, is the "declaration, not
behaviour" shape this port's brief asks to be caught rather than shipped.
What's ported instead is a regression-test module that proves the invariant
those two functions used to enforce by hand now holds structurally: it
deserializes the exact bad wire shape (`"lines": null`, mirroring the TS
test's `binaryFileWithNullLines` fixture) into `crowbar_proto`'s real types
and asserts it degrades to `vec![]` instead of erroring. This was confirmed
to be a real, failable test — not a restatement of the type signature — by
temporarily removing `null_default.rs`'s guard and re-running it; see
`normalize_diff.rs`'s mutation-testing note for the real captured failure.

**Liveness note, independent of the above:** `normalize-diff.ts` has zero
production importers anywhere in the current `web/src` tree — `git grep`
finds only its own test file. Confirmed before deciding how to port it, not
after, per this project's "port only live components" lesson. `lib/persistence/`
— the mechanism the TS bug lived downstream of — is also deleted wholesale by
D6 (spec §5.4) and not part of the native port at all, so even the bug's
original precondition has no Rust-side equivalent to reconstruct.

## 3. `diff-buffer-path.ts` — real logic, but also currently dead in `web/src`

`getDiffBufferFilePath` is ported (unlike `normalize_diff`) because it is
genuine, non-trivial, non-vacuous parsing logic — a wrong implementation is
directly testable and directly fails, which the mutation-tested `.diff`-suffix
edge case demonstrates. But it is worth recording plainly: like
`normalize-diff.ts`, its only importer in the current `web/src` tree is a
test file (`use-git-diff-data.test.ts`), and there is no `use-git-diff-data.ts`
implementation file for that test to be exercising a hook of — confirmed via
`find web/src -iname 'use-git-diff-data*'`. This item ports it per the task's
explicit scope regardless (it names the file, its line count, and its
purpose directly), but the liveness gap is real and stated here rather than
implied away by the fact that the function itself is well-tested.

## 4. Types reused from `crowbar-proto`, and why

Following `crate::workspace`'s precedent (`placeholder.rs`/`branch.rs` reusing
`WorkspaceDTO` instead of a hand-rolled duplicate):

- **Reused directly:** `crowbar_proto::api_v0_dto::GitFileDTO` (for TS
  `GitFile`), `crowbar_proto::domain_git::ReviewFileSummary`,
  `crowbar_proto::domain_git::GitFileStatus`, and
  `crowbar_proto::domain_git::DiffLine` (as `GitDiff::lines`'s element type).
- **Not reused:** `crowbar_proto::domain_git::FileDiff`, for TS `GitDiff` — see
  §1 and `types.rs`'s own doc comment for the optionality mismatch that makes
  this the right call rather than a stylistic one.
- **Not ported:** `MultiFileDiff` (TS `git-diff-types.ts`) — nothing in this
  item's six functions constructs or consumes one; `normalize_diff`'s tests
  exercise `crowbar_proto::domain_git::MultiFileDiff` directly instead.
  `ParsedHunk` (same TS file) — declared, never constructed anywhere in
  `web/src`; dead code upstream, not ported.
- **Not ported (fields):** `types::GitDiff` omits `new_path`, `is_binary`,
  `is_image`, `old_blob_base64`, `new_blob_base64`, `raw_patch` — no function
  in this item's scope ever sets or reads them.

## 5. `git-diff-helpers.ts`'s `getFileStatus` — noted, not ported

`native/mapping/tier-a-denominator.md` §1 flags `getFileStatus`
(`git-diff-helpers.ts`) as a third, smaller near-duplicate of the same
is_new/is_deleted/is_renamed classification this item ports twice already
(`git_status_to_changed_files`, `review_file_summary_to_git_diff`). It is not
in this item's SETUP-defined scope (the six named files) and is not ported
here — recorded so a future item doesn't rediscover it as new territory. The
survey's own recommendation stands: a single `crowbar-core` type should
eventually collapse all three call sites; `types::GitDiff`'s `is_new`/
`is_deleted`/`is_renamed` fields are that type for two of the three already.

## 6. Coverage

`cargo llvm-cov -p crowbar-core`: **100.00% line coverage over 1435 lines**
(up from 787 before this item — the P3.53 workspace-scoping baseline). Every
`git/*.rs` file individually reports 100.00% lines, 0 missed; the crate's one
remaining gap (1 missed *region*, not a missed *line*) is in
`workspace/scope.rs`, pre-existing from P3.53 and out of this item's scope.

130 tests in `crowbar-core`'s lib target (up from 71 before this item: 59 new
`git::*` test cases across the six modules, covering every ported TS test
case plus edge cases the TS suites didn't exercise — see each module's own
`#[cfg(test)]` block for the ported-vs-new split).
