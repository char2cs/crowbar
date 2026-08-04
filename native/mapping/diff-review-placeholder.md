# Placeholder-hunk geometry + file classification (P3.79)

Ports `web/src/features/git/components/diff/review-code-view.tsx`'s embedded
~368-line pure region (roughly lines 93–410: `partitionReviewFiles`,
`buildPlaceholderFileDiff`, and the 7 module-private helpers reachable only
through them or the component) into
`native/crates/crowbar-diff/src/review_placeholder.rs`, plus `getFileStatus`
(`web/src/features/git/utils/git-diff-helpers.ts`) into
`native/crates/crowbar-core/src/git/get_file_status.rs`. The item brief cited
`native/mapping/tier-a-denominator.md`'s §1/§2 crate-boundary finding for
**both** exports landing in one crate; §1 below explains why that finding
actually splits them across two.

## 1. Where each TS name landed, and the crate-boundary correction

| TS name | file | Rust item | crate |
|---|---|---|---|
| `partitionReviewFiles` | `review-code-view.tsx` | [`partition_review_files`] | `crowbar-diff::review_placeholder` |
| `buildPlaceholderFileDiff` | same | [`build_placeholder_file_diff`] | same |
| `trimToPatchCap` (private) | same | `trim_to_patch_cap` (private) | same |
| `reserveAtMost` (private) | same | `reserve_at_most` (private) | same |
| `distributeContext` (private) | same | `distribute_context` (private) | same |
| `buildPlaceholderHunks` (private) | same | `build_placeholder_hunks` (private) | same |
| `buildTailHunk` (private) | same | `build_tail_hunk` (private) | same |
| `patchCacheKey` (private) | same | [`patch_cache_key`] | same |
| `parseSingleFilePatch` (private) | same | [`first_parsed_file`] — **wrapper control-flow only**, see §4 | same |
| `getFileStatus` | `git-diff-helpers.ts` | [`get_file_status`] | **`crowbar-core::git`**, not `crowbar-diff` |
| `getImgSrc` | same | — not ported (presentation, brief's explicit skip) | — |

**The brief's premise did not hold for `getFileStatus`, and this is stated
plainly rather than silently corrected.** The brief said: *"native/mapping/
tier-a-denominator.md's own §1/§2 crate-boundary finding puts this area in
crowbar-diff, not crowbar-core... 2. getFileStatus from git-diff-helpers.ts —
genuine classification logic."* Reading §2 closely (not just citing it)
finds the opposite grouping for this one function. §2's own "Finding"
paragraph enumerates three things easy to mistake for "diff algebra":

1. *"File-status classification (is_new/is_deleted/is_renamed → label) —
   genuine, tiny, **git-model** logic, counted in §1."*
2. *"Placeholder hunk-geometry estimation... its purpose is virtualiser
   sizing, i.e. crowbar-diff-crate logic, not crowbar-core."*
3. *"Viewport windowing/materialisation... and diff-text search... pure,
   well-tested, but crowbar-diff-crate logic, not crowbar-core."*

Only (2) and (3) are assigned to `crowbar-diff`. (1) is explicitly called
git-model logic and, in §1's own "genuine, portable git-model logic" list,
`getFileStatus` is named directly, alongside `gitStatusToChangedFiles` and
`reviewFilesSummaryToChangedFiles` — both **already ported** into
`crowbar-core::git` — with the note *"Three near-duplicate implementations
of 'classify a file's change kind' is itself a finding: a single
`crowbar-core` type should collapse all three."* So `getFileStatus` lands
beside its two siblings in `crowbar-core::git::get_file_status`, operating
directly on the already-ported `crowbar_core::git::GitDiff` type (which
already carries `is_new`/`is_deleted`/`is_renamed` — the only three fields
this function reads), not in `crowbar-diff`. No consolidation of the three
near-duplicate classifiers was attempted — that is the doc's own separate
finding, not this item's scope.

Placeholder-hunk geometry (`buildPlaceholderFileDiff` and its five
computational helpers) and `partitionReviewFiles` **are** `crowbar-diff`
material, per (2) above and confirmed against the design spec directly
(`docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md`
§4.2's crate-contracts table — this crate's coverage-gate column reads
*"logic ≥98%; view via oracle"*, the concrete example that column was
written for). **This does contradict two other documents**, stated rather
than left standing silently:

- `crowbar-diff/src/lib.rs`'s own scaffold doc comment (item 0.1, written
  before any of §12's logic partition had a concrete example): *"Keep the
  algebra in `crowbar-core`... keep this crate to rendering and
  interaction."* Corrected in this item's first commit, citing this doc.
- Spec §4.2's crate-contracts table itself, whose `crowbar-core` "Owns"
  column lists *"diff algebra"* with no carve-out for placeholder/windowing
  logic. The mapping doc's later, per-function analysis is what the brief
  explicitly directs following over the table's literal wording, and this
  item does so — but the disagreement is real, not resolved, and is spelled
  out here for whoever next touches either document.

## 2. The dependency contract, and the re-export it required

`crowbar-diff`'s §4.2 contract is `ui`/`state`/`core` — **not**
`crowbar-proto` directly, confirmed against the spec's crate-contracts table
("the operative one" per its own 2026-07-30 correction note), not just the
scaffold's paraphrase. But `partition_review_files`/`build_placeholder_file_diff`
need `crowbar_proto::domain_git::{FileDiff, FileOutline, HunkShape}` as input
types (see §3). Adding `crowbar-proto` directly to `crowbar-diff/Cargo.toml`
would be exactly the kind of edge the workspace `Cargo.toml`'s own comment
calls *"a spec violation, not a style question."*

Resolved the same way `crowbar-ui` already resolves an identical problem for
`gpui` (`pub use gpui;`, cited in `native/README.md` as deliberate — "a
framework bump is then one edit in the design system rather than one per
crate"): `crowbar-core::git` now carries

```rust
pub use crowbar_proto::domain_git::{FileDiff, FileOutline, HunkShape};
```

so `crowbar-diff` reaches them as `crowbar_core::git::{FileDiff, FileOutline,
HunkShape}` with no new manifest edge. This is the one shared-file touch
outside `crowbar-diff` itself, kept minimal and additive (three re-export
lines, a doc comment, and the new `get_file_status` module + its two `pub
mod`/`pub use` lines) — the concurrently-worked sibling item is
`crowbar-core::review` (a different module), not `crowbar-core::git`, so
there is no collision.

## 3. The `@pierre/diffs` retyping — where the shapes diverge, what was assumed

**Input types — clean reuse, no local duplication:**

- `HunkShape`/`FileOutline` (`review-window-api.ts`'s own hand-written TS
  types, not `@pierre/diffs`'s) already matched `crowbar_proto::domain_git`'s
  generated types field-for-field before this item touched anything —
  `old_start`/`old_lines`/`new_start`/`new_lines`, `path`/`old_path`/`hunks`/
  `is_partial`/`is_binary`, exactly.
- `GitDiff` (`git-types.ts`) maps onto `crowbar_proto::domain_git::FileDiff`
  closely, with one real divergence: TS's `additions?`/`deletions?`/
  `uncommitted?` are optional, `FileDiff`'s are non-optional `i64`/`bool`.
  **Assumption stated in the module doc:** this function's real caller path
  (the branch-review summary) always has counts, so the "absent" state TS's
  `?` encodes is not reachable through this function in practice; `count_of`
  only has to fold the `-1` binary sentinel to `0`, not also fold
  `undefined` — and both fold to the same output, so the divergence would not
  be observable even if a future caller violated the assumption.

**Output types — the actual point of the "retype against crowbar-proto"
instruction, and where it does not work verbatim:**

`crowbar_proto::domain_git::Hunk` is `{hunk_id, header, start_line,
end_line}` — an index/reference into an *already-parsed* `FileDiff.lines`,
used to jump to a hunk by id. It carries no row-count or rendering-geometry
field at all and cannot represent "how tall does this hunk render," which is
the entire subject of `buildPlaceholderHunks`. Forcing the algorithm's output
through it would mean inventing fake `hunk_id`/`header` values with no real
meaning — exactly the silent-coercion class of bug the brief's
directory-vs-trailing-slash example warns about. So [`PlaceholderHunk`]/
[`PlaceholderFileDiff`]/[`ChangeKind`] are **new, local types** in
`crowbar-diff`, sized from exactly the fields this algorithm reads or writes
of `@pierre/diffs`'s `Hunk`/`FileDiffMetadata`/`ChangeTypes` — not copied
from those libraries' full public surface. One field-level note: `hunkContent`
(always `[]` in every hunk this module constructs, exercised by zero TS
tests) was dropped from `PlaceholderHunk` entirely; `additionLines`/
`deletionLines` (also always `[]`, but explicitly asserted by an existing TS
test) were kept on `PlaceholderFileDiff`.

## 4. `parseSingleFilePatch` — crate survey, done before hand-rolling anything

Raised mid-item by the coordinator: the brief's "retype against
crowbar-proto" covered types, not the *parser* `parseSingleFilePatch` wraps
(`@pierre/diffs`'s `parsePatchFiles`, real unified-diff-text parsing). Same
discipline as the `.gitignore` item's `ignore`-crate decision
(`core-gitignore.md` §2) — survey before hand-rolling.

| crate | license | last release (2026-08) | git-format fit |
|---|---|---|---|
| `patch` (crates.io) | MIT | 0.7.0, Dec 2022 — ~4 years stale | "forgiving" of git's extra headers by *ignoring* them; no structured rename/no-EOF-newline/binary output |
| `gitpatch` (maintained fork, same API) | MIT | 0.7.1, Apr 2025 | same gap as `patch` despite the name — still discards rather than models git's extended headers |
| `diffy` | MIT OR Apache-2.0 | 0.5.1, Jul 2026 — active | `patch_set` module + `ParseOptions::gitdiff()`: `FileOperation::{Rename{from,to}, Copy{from,to}, ...}` populated only from git's extended headers, streaming multi-file iterator |

**Zed checked too** (spec §5.2's reference for this surface) and turns out
not to be a precedent: Zed's `buffer_diff` crate computes a diff between a
live buffer and a base-text blob it already holds — the same job
`imara-diff`/`similar` do — and never parses *serialised* patch text, because
Zed always has both sides of the content in memory. Crowbar's daemon, by
contrast, hands the client an already-`git diff`-rendered patch **string**
(`GET /review/patch`), so this is a real parsing problem Zed's own
architecture does not have. This is a genuine finding, not a dead end: it
means "check what Zed does" resolves to "Zed doesn't have this problem,"
which is itself worth recording so the next person doesn't re-ask the
question.

**Recommendation for whoever builds the real (non-placeholder) parser:
`diffy`'s `patch_set` module.** Not `patch`/`gitpatch` — their whole strategy
is discarding exactly the git-specific structure (`FileDiffMetadata.type`,
`Hunk.noEOFCRDeletions`/`noEOFCRAdditions`) a real integration needs, so
adopting either would still require re-scanning the raw text by hand for the
headers the crate throws away. **One real semantic divergence found, not
assumed away** (the same class of gap the `.gitignore` item found for
`isPathGitIgnoredByFileTreeRules` vs. `Gitignore::matched`'s `is_dir` bool):
`@pierre/diffs`'s `Hunk` carries `noEOFCRDeletions`/`noEOFCRAdditions` as
**explicit booleans**; `diffy`'s `Line` instead carries the terminating `\n`
(or its absence) as part of each line's own bytes. A real integration must
derive the two booleans by checking that fact on the last removed/added line
of each hunk — defaulting them to `false` without doing that check would
silently misreport every no-trailing-newline file. `diffy` also has no
`Copy` counterpart in `ChangeTypes`; flagged, not resolved (Crowbar's daemon
is not known to emit a bare copy-only diff for this surface).

**Not implemented here.** Building the real `FileDiffMetadata` construction —
running `diffy::patch_set` end to end, tokenising hunk content, deriving the
two EOF-newline booleans — is separate, larger surface than this item's
placeholder-*geometry* scope (sizing a file *before* its real patch
arrives), the same way `patch-window.ts` and `diff-search.ts` were already
carved out as separate items. `diffy` is **not** added to
`crowbar-diff/Cargo.toml` by this item — a manifest edge with no caller to
justify it. [`first_parsed_file`] ports only `parseSingleFilePatch`'s actual
own logic: try each already-parsed batch in order, take its first file, stop
at the first hit, treat a failed parse as "no metadata" — generic over an
already-produced parse result, not over a parser.

## 5. Mutation testing — 5 mutations, ≥2 on `distribute_context`/`build_placeholder_hunks`

All five applied by editing the source directly (no `cp`/`mv` backup
round-trip, so no mtime staleness risk), run to a real failure, output
captured below, then reverted and re-verified green.

1. **`build_placeholder_hunks` — dropped the `.min(shape.new_lines)` clamp
   on `shared`.** All 39 tests then passing was itself the finding: the
   original `build_hunks_clamps_shared_context_to_the_hunk_own_side_minimums`
   test used a symmetric hunk (`old_lines == new_lines`), so the missing
   clamp was invisible — a real gap in the first-pass test, caught by the
   mutation rather than by inspection. Added an asymmetric case
   (`old_lines=5, new_lines=2`) before re-testing:
   ```
   thread '...::build_hunks_clamps_shared_by_the_smaller_side_when_old_and_new_differ' panicked:
   assertion `left == right` failed
     left: -3
    right: 0
   ```
   Reverted; new test kept permanently (it is the one that actually exercises
   this clamp).

2. **`distribute_context` — flipped the early-return guard `cap_total == 0
   || deletions == 0` to `&&`.**
   ```
   thread '...::distribute_context_is_all_zero_when_deletions_is_zero_even_with_room' panicked:
   assertion `left == right` failed
     left: [5]
    right: [0]
   ```
   Reverted.

3. **`reserve_at_most` — changed the `room <= 0` guard to `room < 0`**
   (off-by-one at the `room == 0` boundary).
   ```
   thread '...::reserve_at_most_leaves_a_hunk_unchanged_when_room_is_non_positive' panicked:
   assertion `left == right` failed
     left: PlaceholderHunk { ..., split_line_count: 1, unified_line_count: 0, ... }
    right: PlaceholderHunk { ..., split_line_count: 3, unified_line_count: 6, ... }
   ```
   Reverted.

4. **`trim_to_patch_cap` — changed the break condition `>` to `>=`.** No
   existing test hit the exact boundary (`unified + count == cap`); added
   `trim_keeps_a_hunk_that_lands_exactly_on_the_cap` first, confirmed it
   caught the mutation:
   ```
   thread '...::trim_keeps_a_hunk_that_lands_exactly_on_the_cap' panicked:
   assertion `left == right` failed
     left: 1
    right: 2
   ```
   Reverted; new test kept permanently.

5. **`get_file_status` — swapped `is_new`/`is_deleted` precedence order.**
   ```
   thread '...::is_new_wins_over_every_other_flag' panicked:
   assertion `left == right` failed
     left: "deleted"
    right: "added"
   ```
   Reverted.

All five reverts confirmed by a full re-run of both crates' test suites
(328 + 44 passed, 0 failed) after every revert.

## 6. Tests — ported vs. authored

44 tests in `crowbar-diff::review_placeholder::tests`, 6 in
`crowbar-core::git::get_file_status::tests`.

- **5 ported directly** from
  `web/src/__tests__/features/git/components/diff/review-code-view.test.tsx`:
  `partitionReviewFiles`'s 2 cases (order-preservation, binary/image
  routing) and `buildPlaceholderFileDiff`'s 3 cases (geometry-from-shapes,
  partial-file topping-up, capped-but-true-counts).
- **39 authored for this port**, covering every branch enumerated in
  `review_placeholder.rs`'s own module doc before any Rust was written
  (`distribute_context`'s 4 branches, `build_placeholder_hunks`'s clamping
  and running-position accumulation, `build_tail_hunk`'s empty-vs-populated
  default, `reserve_at_most`'s two guard branches and the floor-at-1,
  `trim_to_patch_cap`'s prefix-keep vs. single-hunk-scale-down, the
  three-gate "top up from summary" block, `first_parsed_file`'s
  index-0-only/failed-parse/all-empty cases, `patch_cache_key`, `round_ratio`
  against JS's half-up `Math.round`) plus the two tests the mutation pass
  itself proved were missing (§5.1, §5.4).
- **6 authored for `get_file_status`**, since — per
  `tier-a-denominator.md` §1's own "Tests" table — *"Zero test files exist
  for `git-diff-helpers.ts`'s `getFileStatus`"*: nothing to port.

## 7. Coverage — measured directly, before and after

`cargo llvm-cov`, run against a temporary `git worktree add --detach` at the
parent commit (802fa0f3) for "before," removed immediately after measuring:

| crate | before | after |
|---|---|---|
| `crowbar-diff` (whole crate — scaffold-only before this item) | 0 lines instrumented | 648 lines, **100.00%** line coverage (99.90% region, 100% function) |
| `crowbar-core` (whole crate) | 6,457 lines, 99.33% (43 missed) | 6,504 lines, 99.34% (43 missed — unchanged; all new code fully covered) |

The three lines `cargo llvm-cov --show-missing-lines` initially flagged in
`review_placeholder.rs` (`is_image_path`'s no-extension branch,
`round_ratio`'s defensive zero-denominator guard, `first_parsed_file`'s
all-batches-empty case) were closed with targeted tests before the number
above, not left as an unexplained gap.

## 8. Gates — all green in the foreground

- `cargo clippy --workspace --all-targets -- -D warnings` — clean. Fixed
  4 pedantic findings along the way: `doc_markdown` (two unbacktick'd
  identifiers in `get_file_status`'s module doc), `similar_names`
  (`patch_cache_key`'s `patch`/`path` params — renamed to `patch_text`),
  `map_unwrap_or` (`partition_review_files`'s binary-flag fallback, now
  `map_or_else`), `collapsible_if` (the tail-hunk gate, now a chained
  `if let ... && ... && ...`).
- `cargo test --workspace` — **2,472 passed, 0 failed** (trunk baseline
  2,422 + this item's 50 new tests — 44 + 6 — exactly accounts for the
  difference, confirming the branch point matched the stated baseline).
- `bash native/scripts/check-invariants.sh` — **7/7**. Rule 5 (`cargo fmt
  --check`) initially failed, scoped entirely to `review_placeholder.rs`
  (confirmed via `git status` before running `cargo fmt`, so the fix did not
  risk absorbing drift from any other file); re-ran clean after.

## 9. What this item found, checked against the brief's premises

- **The crate-boundary premise was wrong for `getFileStatus`.** The brief
  cited tier-a-denominator.md's §1/§2 finding as covering both
  `getFileStatus` and the placeholder-geometry region under one
  `crowbar-diff` verdict; §2's own "Finding" paragraph and §1's own export
  list say the opposite for `getFileStatus` specifically. See §1 above.
- **"Retype against crowbar-proto's Hunk" does not work verbatim for the
  output types**, and saying so plainly (rather than silently inventing a
  fake `hunk_id`/`header`) is the point of §3. It works cleanly for the
  input types (`HunkShape`/`FileOutline`/mostly `FileDiff`), which the brief
  did not separately call out but which turned out to be the easy half.
- **The dependency-contract scaffold text and the crate-boundary scaffold
  text were both first-guess placeholders that this item's own sources
  (the spec's crate-contracts table; the mapping doc's later per-function
  pass) superseded** — both corrected in `lib.rs` rather than left
  contradicting the code that now exists beside them (§1, §2).
- **The `parseSingleFilePatch` gap was real** — the brief's own scope list
  named it as one of the 7 helpers to port "as one unit," but doing so
  faithfully would have meant depending on `@pierre/diffs`'s own parsing
  semantics, which the same brief forbids. Resolved by porting only the
  wrapper's actual control flow and doing the crate survey the coordinator's
  follow-up asked for (§4) rather than silently either skipping the function
  or hand-rolling a parser.
- **One first-pass test was genuinely vacuous** for the exact clamp a
  mutation was designed to test (§5.1) — caught by the mutation pass itself,
  which is the reason this project runs mutations rather than trusting green
  coverage numbers alone.
