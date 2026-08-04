# `crowbar-core::review` — the review-thread area (P3.78)

The `crowbar-core` half of `native/mapping/tier-a-denominator.md` §7 ("Review
threads"), ported into `native/crates/crowbar-core/src/review/`, the sixth
Tier A area to land after workspace scoping (P3.53), the git model (P3.67),
keymap resolution, settings, and the file-tree model. §7 is, in its own
words, "the one area where the export audit found nothing to prune" — all
900 TS lines across its four non-component files are LIVE or CONDITIONAL,
zero DEAD, zero TEST-ONLY. Scoped from §7's per-export table (~line 2069)
and its "port, no skips found" recommendation (~line 2227), not from the
surrounding prose or file-level rows, per this item's own brief.

| module | ported from | TS lines | Rust `wc -l` |
|---|---|---|---|
| `state.rs` | `features/workspace/stores/slices/branch-review-slice.ts` | 156 | 508 |
| `api.rs` | `features/git/api/review-api.ts` (2 of 18 exports, plus 2 private helpers) | 288 (whole file; ported surface is smaller) | 523 |
| `annotation.rs` | `features/git/components/diff/use-review-annotations.tsx` (10 of 11 exports) | 395 (whole file; ported surface is smaller) | 321 |
| `mod.rs` | (new — index + scope rationale) | — | 67 |

The Rust files run larger than their TS sources for the same reason every
prior area's do: doc comments citing the source, the reasoning behind every
divergence, ported-vs-new test provenance, and (below) real mutation-testing
transcripts pasted verbatim — none of which the TS originals carry. The
`cargo llvm-cov`-measured *executable* line counts (§4 below) are smaller
than `wc -l` and are what the coverage gate actually measures.

## 1. What each module models

- **`state::{ReviewMessage, ReviewThread, ReviewConversation, DiffStatus,
  BranchReviewState}`** — the four TS types plus the state shape, ported
  verbatim. Twelve TS mutators (`createBranchReviewSlice`'s closure-bound
  methods) become 11 free functions — see §2.
- **`api::{map_thread, build_review_state}`** — the two pairs the brief
  names, `mapThread`+`mapReply` and `getReview`+`mapConversation`, each
  ported as one unit (their private helpers are unreachable except through
  the public partner, matching the TS source). `getReview` is split: only
  its pure reshape survives as `build_review_state`; the `apiFetch` call is
  transport, left for `crowbar-client`. See `api.rs`'s own module doc for
  two checked-not-assumed findings this produced (§3 below).
- **`annotation::{is_draft_thread, to_annotation_side, to_thread_side,
  thread_to_annotation, annotation_to_thread, group_annotations_by_path,
  count_threads_by_path, DiffLineAnnotation<T>, ReviewAnnotation,
  DRAFT_THREAD_ID}`** — the pure-helper half of `use-review-annotations.tsx`.
  `DiffLineAnnotation<T>` is a native, `@pierre/diffs`-free reimplementation
  — see that module's doc for exactly what was assumed about its shape.

## 2. Decision: the 12 mutators, extracted into 11 free functions

TS's `createBranchReviewSlice` is a Zustand `StateCreator` — a factory
closure returning bound mutator methods, each a trivial pure
`BranchReviewState -> BranchReviewState` transition wrapped in an Immer
`set(...)` call. The survey's prose calls these "12 pure mutators", but they
are not separately exported TS symbols — properties of one factory's return
value, so §7's export-liveness table (correctly) audits the whole factory as
one LIVE unit and cannot verify the 12 individually. The brief leaves the
shape decision to the porter.

**Decision: extract each into a standalone `fn(&mut BranchReviewState, ..)`
free function**, not a struct-of-closures or trait mirroring
`BranchReviewSlice`. Reasoning:

- `crowbar-core` has no reactive shell (no Zustand, no Immer, no `set`) —
  that belongs to whatever Phase-4/`crowbar-state` code eventually owns a
  `BranchReviewState` behind a real store. Reproducing the `StateCreator`
  closure shape here would mean inventing a shell this crate is not supposed
  to own, just to hold logic that doesn't need one.
- Free functions are exactly what every mutator's body already is once the
  `set(...)` wrapper and Immer draft-mutation sugar are stripped away — no
  behavior is invented, only the reactive packaging is dropped.
- Keeping a *consistent* API surface mattered more than cherry-picking which
  of the 12 "look interesting": some (`revealBranchReviewFile`,
  `addReviewMessage`, `upsertReviewThread`) carry genuine multi-step logic;
  others (`setBranchReviewDescription`) are a single field write. Porting
  only the former as functions and leaving the latter as bare field access
  would give Phase-4 code two different contracts for one state struct.

Result: 11 functions in `state.rs` (`set_branch_review_description`,
`set_branch_review_merge_strategy`, `set_branch_review_diff_status`,
`reveal_file`, `add_review_thread`, `remove_review_thread`,
`add_review_message`, `set_review_thread_resolved`, `upsert_review_thread`,
`set_branch_review_conversations`, `add_review_conversation`) — 12 minus
`resolveReviewThread` (§3).

## 3. Decision: `resolveReviewThread` is not ported

Its own TS doc comment already marks it `@deprecated` ("Kept for backward
compat... Use setReviewThreadResolved instead"). Checked, not just taken on
faith:

- **Behavior**: identical to `set_review_thread_resolved(state, id, true)` —
  find the thread by id, `if (t) t.isResolved = true`. Confirmed by reading
  both bodies side by side, not inferred from the doc comment's own claim.
- **Callers**: `git grep -n resolveReviewThread web/src`, outside the
  slice's own definition and its one dedicated legacy-compat test, returns
  zero. The deprecation is not aspirational — nothing calls it.

Its reason to exist in TS is a JS/store-API concern this port does not
inherit: a Zustand store's public shape may be called by code the repo
cannot see (a stale closure, a devtools extension, a saved macro), so
removing a method is a breaking change even at zero known callers.
`crowbar-core::review` has no such external surface yet — nothing outside
this same workspace can call a Rust function that does not exist yet — so
carrying a callable-but-never-called backward-compat shim forward would be
manufacturing dead code on day one, against this port's stated preference
for live code paths (`crate::git`'s module doc on the unconstructed
`ParsedHunk`, same reasoning). **Not ported.** Its one dedicated TS test
("resolveReviewThread (legacy) still sets isResolved to true") is not
ported either; the behavior it exercised is covered by
`set_review_thread_resolved_true_marks_thread_resolved`.

## 4. Findings — premises checked against the tree, not assumed

Three, all found by reading the Go backend directly rather than trusting the
TS types, none previously recorded in `native/mapping/`.

### 4.1 `ReviewState.diff` (`MultiFileDiff`) has no producer and no consumer, on either side of the wire

TS `WireBranchReview`/`ReviewState` both carry a non-optional
`diff: MultiFileDiff` field. Not ported. Two independent confirmations:

- **Backend**: `api/internal/domain/branch_review.go`'s `BranchReview`
  struct is `{Description, MergeStrategy, Threads}` — no diff field at all
  — and `api/internal/api/v0/dto/review.go`'s `BranchReviewDTO` (what `GET
  /v0/workspaces/:wsId/review` actually serialises, via
  `BranchReviewDTOFrom`, the endpoint's only call site) matches it exactly.
  `raw.diff` is `undefined` on every real response.
- **Frontend**: `branch-review-pane.tsx`'s own comment, at its one call
  site: *"Deliberately NOT storing review.diff. The pane renders from the
  files summary + outline now; keeping the composite's line-level diff would
  re-import the very payload this phase removed."*

Same "declaration, not behaviour" shape `crate::git`'s module doc already
declines to port for `types::GitDiff`'s six unused `FileDiff` fields —
independently re-confirmed here for a different field, in a different area.

### 4.2 `conversations` used to be on the wire; a backend regression test proves it no longer is

Kept (per §7's explicit "port each pair as one unit" scope, and because
`state::BranchReviewState::conversations` and its two mutators are
independently LIVE store surface this item ports regardless), but recorded
as a finding rather than silently ported without comment:
`api/internal/api/v0/dto/review_test.go`'s
`TestBranchReviewDTOFromHasNoConversationsKey`, whose own comment reads
*"guards the Chat-aggregate removal (decision 2): the branch-review wire
model must no longer carry a `conversations` key. The frontend degrades to
`raw.conversations ?? []`, so its absence is FE-safe"* — proves the backend
deliberately stopped sending it. So `map_conversation` is reachable in this
port exactly as it is in the TS source, but nothing in the *current* backend
contract can ever hand it a real value. Not caught by
`tier-a-denominator.md`'s export-liveness method, which traces frontend
reachability, not backend wire truth.

### 4.3 `ThreadDTO.side`'s real-world domain is checked, not inferred from the TS union type

TS `ThreadDTO.side: 'old' | 'new'` is a compile-time-only promise
`mapThread` never validates at runtime — it just copies `t.side` through.
The generated Rust `ThreadDTO.side` is a plain `String`, so this port has to
decide what an unexpected wire value means. Checked directly rather than
guessed:

`api/internal/domain/review_side.go` declares `type ReviewSide string` with
exactly two constants, `ReviewSideLeft = "left"` / `ReviewSideRight =
"right"` — a *different* vocabulary than "old"/"new". `git grep -n
'ReviewSideLeft\|ReviewSideRight' api/internal` finds both used **only
inside `_test.go` files** — zero production callers anywhere in the write
path (`threads.go`'s request binding through the `reviewthread` repository
all pass the client's JSON string through unvalidated: `Side: c.Side` /
`Side: in.Side`, no coercion). Since the only real client is this frontend,
and `OpenThreadInput.side: 'old' | 'new'` is the only thing that ever
originates a value, "old"/"new" is genuinely the entire real-world domain of
this field end to end — the Go domain type's own "canonical" constants name
a vocabulary no production code path ever actually constructs. `parse_side`
treats anything other than `"old"` as `New`, matching `toAnnotationSide`'s
own fallback shape, now for a case confirmed unreachable with real data
rather than merely assumed to be.

## 5. `@pierre/diffs`'s `DiffLineAnnotation<T>` and `ReviewAnnotationLayer`

Per the brief's known entanglement: `DiffLineAnnotation<T>` is retyped
natively in `annotation.rs` (`crowbar-core` cannot depend on a UI-facing
diff-rendering package). What was assumed about its shape: every
construction site in the ported TS source and its test suite uses exactly
three fields (`side`, `lineNumber`, `metadata`); no other field is ever read
or written. `annotation::DiffLineAnnotation<T>` carries exactly those three.

The brief's scope list also names `ReviewAnnotationLayer` alongside the
seven pure functions. Checked against the source rather than ported as a
literal struct: it is `useReviewAnnotations`'s own return type, imported
nowhere else in `web/src` (`git grep -n ReviewAnnotationLayer web/src` finds
only its declaration and the hook's signature). Of its 11 fields, 2
(`annotationsByPath`, `threadCounts`) are exactly the return values of
`group_annotations_by_path`/`count_threads_by_path` — already covered,
nothing left to add a type for — and the other 9 are `useState`-backed
values, transport-bound closures, or `ReactNode` renderers: the hook-body
surface the brief separately excludes. Not ported as a struct; recorded here
rather than silently dropped, since the brief named it explicitly.

## 6. Tests

47 tests, all in `#[cfg(test)]` modules alongside the code they exercise —
30 ported from the three existing TS suites
(`branch-review-slice.test.ts`, `review-api.test.ts`,
`use-review-annotations.test.tsx`'s pure-helper cases), 17 new (documented
per-file, split by a `// --- ported from ... ---` / `// --- new ... ---`
comment divider matching this crate's existing convention). One TS test
case (`resolveReviewThread` legacy) was deliberately not ported — §3.

**Adaptations, not straight ports, recorded where they happened:**

- `review-api.test.ts`'s `getReview` test asserted both the fetch URL and
  the returned shape; only the shape half survived (`get_review_reads_mapped_merge_strategy_and_returns_threads_empty`)
  since the fetch itself is not in scope.
- `use-review-annotations.test.tsx` asserts `annotation.metadata` `.toBe(thread)`
  (JS reference identity) once; Rust has no equivalent for an owned value
  type, so the ported test (`thread_to_annotation_carries_the_line_number_and_the_thread_as_metadata`)
  asserts value equality instead, with a comment saying so.

### Mutation testing — 5 run, all caught, output pasted, all reverted via `git checkout`

Per the brief's mtime-trap warning, every revert used `git checkout --
<file>` (a fresh write, not an `mv` from a `cp` backup) and was followed by
an explicit `touch` + fresh `cargo test` run to rule out a stale binary.

**1. `state::upsert_review_thread` — always push instead of replacing an existing id:**
```
match state.threads.iter().position(|t| t.id == thread.id) {
    Some(_idx) => state.threads.push(thread),   // was: state.threads[idx] = thread
    None => state.threads.push(thread),
}
```
```
thread 'review::state::tests::upsert_review_thread_merges_replaces_by_id_when_thread_already_exists' panicked at crates/crowbar-core/src/review/state.rs:318:9:
assertion `left == right` failed
  left: 2
 right: 1
test result: FAILED. 20 passed; 1 failed
```

**2. `state::remove_review_thread` — inverted retain predicate:**
```
state.threads.retain(|t| t.id == thread_id);   // was: t.id != thread_id
```
```
---- remove_review_thread_is_a_no_op_for_an_unknown_id ----
assertion `left == right` failed
  left: 0
 right: 1
---- remove_review_thread_drops_only_the_matching_id ----
assertion `left == right` failed
  left: ["a"]
 right: ["b"]
test result: FAILED. 0 passed; 2 failed
```

**3. `api::map_thread` — `is_resolved` hardcoded to `false`:**
```
is_resolved: false,   // was: t.resolved
```
```
thread 'review::api::tests::derives_is_resolved_from_resolved_true' panicked at crates/crowbar-core/src/review/api.rs:325:9:
assertion failed: map_thread(&dto).is_resolved
test result: FAILED. 12 passed; 1 failed
```

**4. `annotation::to_annotation_side` — swapped the side map (the exact
correctness-critical inversion both the TS and Rust doc comments warn
about):**
```
ThreadSide::Old => AnnotationSide::Additions,   // was: Deletions
ThreadSide::New => AnnotationSide::Deletions,   // was: Additions
```
```
test result: FAILED. 6 passed; 5 failed
  (maps_a_threads_old_side_to_deletions_and_its_new_side_to_additions,
   places_an_old_side_thread_on_the_deletions_half,
   thread_to_annotation_carries_the_line_number_and_the_thread_as_metadata,
   round_trips_an_annotation_through_a_thread_and_back_on_both_sides,
   groups_annotations_under_each_thread_file_path_ordered_by_line)
```

**5. `annotation::group_annotations_by_path` — dropped the `sort_by_key` call:**
```
for (path, list) in by_path {   // was: for (path, mut list) in by_path { list.sort_by_key(...); ... }
```
```
thread 'review::annotation::tests::groups_annotations_under_each_thread_file_path_ordered_by_line' panicked at crates/crowbar-core/src/review/annotation.rs:271:9:
assertion `left == right` failed
  left: [9, 3]
 right: [3, 9]
test result: FAILED. 0 passed; 1 failed
```

All five reverted; `cargo test -p crowbar-core review::` back to 47/47
green after each, and 47/47 again after a final combined `touch` + rerun.

## 7. Coverage

`cargo llvm-cov -p crowbar-core`, measured directly both sides (before: a
throwaway `git worktree add` pinned at this item's parent commit,
`802fa0f3`; after: this branch, post-fmt, post-coverage-gap-fix):

| | lines | missed | line coverage |
|---|---|---|---|
| **Before** (parent commit, no `review` module) | 4,319 | 11 | 99.75% |
| **After** (this item) | 4,982 | 11 | 99.78% |
| **`review` module alone** | 663 | 0 | **100.00%** |

The crate-wide missed-line count is unchanged at 11 (all pre-existing, in
`file_tree/gitignore.rs`, `file_tree/git_status.rs`,
`file_tree/visible_rows.rs`, `keymap/chord.rs`,
`settings/normalization.rs`, `workspace/scope.rs` — none of them touched by
this item). `review/api.rs` briefly carried 2 missed lines (the empty-author
`None` fallback in both `map_reply` and `map_thread`'s root message, neither
exercised by the existing fixtures) — closed with two new tests
(`root_message_author_falls_back_to_none_when_wire_author_is_empty`,
`reply_author_falls_back_to_none_when_wire_author_is_empty`) rather than
left as an unexplained gap.

Per-file (`cargo llvm-cov report`, executable lines):

| file | lines | missed | coverage |
|---|---|---|---|
| `review/state.rs` | 269 | 0 | 100.00% |
| `review/api.rs` | 267 | 0 | 100.00% |
| `review/annotation.rs` | 127 | 0 | 100.00% |

## 8. Gates

All run in the foreground, one at a time, per the brief:

- `cargo clippy --workspace --all-targets -- -D warnings` — clean.
- `cargo test --workspace` — **2,469 passed, 0 failed** (trunk baseline
  2,422 + 47 new `review::*` tests — exact arithmetic match, confirmed by
  summing every `test result:` line in the run, not eyeballed).
- `bash native/scripts/check-invariants.sh` — **7/7 ok**, exit 0. One
  intermediate run caught a real `cargo fmt` drift in all three new files
  (rule 5) — `cargo fmt -p crowbar-core` fixed it and touched no file
  outside `review/`, confirmed by `git diff --stat` before committing the
  fix; all three gates re-run clean afterward.
