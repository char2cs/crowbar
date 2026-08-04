//! File classification and placeholder-hunk-geometry estimation for the
//! windowed Branch Review surface.
//!
//! Ported **as one unit** from the embedded ~368-line pure region of
//! `web/src/features/git/components/diff/review-code-view.tsx` (roughly
//! lines 93–410: the `// ── File classification ──`, `// ── Placeholder
//! geometry ──` and `// ── Patch parsing ──` banners, up to but not
//! including the component itself). The region has exactly two exported
//! entry points — `partitionReviewFiles` ([`partition_review_files`]) and
//! `buildPlaceholderFileDiff` ([`build_placeholder_file_diff`]) — plus seven
//! module-private helpers reachable only through them (or, for two of the
//! seven, only through the component — see "What was **not** ported"
//! below). `native/mapping/tier-a-denominator.md`'s own export-level audit
//! (§"Export-level liveness", §2's per-export table) found the same thing by
//! reading `review-code-view.tsx` directly: only `partitionReviewFiles` and
//! `buildPlaceholderFileDiff` carry an `export` keyword; the rest are
//! unreachable except by calling into those two (or the component), so they
//! cannot be individually imported and are not individually skippable.
//!
//! # Crate and dependency placement — read before assuming either matches the item brief literally
//!
//! **Crate:** `crowbar-diff`, per the mapping doc's own §1/§2 crate-boundary
//! finding — placeholder-hunk-geometry estimation exists to size the
//! windowed review renderer's virtualiser, which is `crowbar-diff`-crate
//! logic (§4.2/§12's own "diff(logic) ≥98%" partition), not `crowbar-core`
//! "diff algebra" (which §2's own text narrows to "hunk/line structure," not
//! rendering geometry). **This directly disagrees with `crowbar-diff`'s own
//! scaffold doc comment** (`lib.rs`, item 0.1: "Keep the algebra in
//! `crowbar-core` where the line gate already applies, and keep this crate
//! to rendering and interaction") **and with spec §4.2's crate-contracts
//! table**, which lists "diff algebra" under `crowbar-core`'s "Owns" column
//! with no carve-out for placeholder/windowing logic. Both of those are the
//! earlier, less granular framing; the mapping doc's later, per-function
//! analysis is what this item's brief explicitly directs following, and this
//! module does. The scaffold comment (`lib.rs`) has been corrected to match
//! and cites this module for the reasoning, so the contradiction is not left
//! silently standing in two places.
//!
//! **`getFileStatus`, the item brief's other named export, is NOT in this
//! module or this crate.** The brief cites the same "§1/§2 crate-boundary
//! finding" for both `getFileStatus` and this placeholder-geometry region,
//! but the mapping doc's own text does not actually place them together:
//! §2's "Finding" paragraph calls file-status classification "genuine, tiny,
//! **git-model** logic, counted in §1," and §1's own "genuine, portable
//! git-model logic" list names `getFileStatus` directly, grouped with the
//! two near-duplicate classifiers already ported into `crowbar-core::git`.
//! Only placeholder-hunk geometry and the windowing/search cluster are
//! assigned to `crowbar-diff`. `getFileStatus` is ported at
//! `crowbar_core::git::get_file_status` instead — see that module's doc for
//! the full citation trail. This is a correction to the brief, not a silent
//! reinterpretation: it groups an unrelated function under this citation
//! without the citation actually covering it.
//!
//! **Dependency:** `crowbar-diff`'s §4.2 contract is `ui`/`state`/`core`, not
//! `crowbar-proto` directly (confirmed against
//! `docs/superpowers/specs/2026-07-30-rust-native-desktop-port-design.md`
//! §4.2's table, "the operative one" per that document's own 2026-07-30
//! correction note — the scaffold's paraphrase of the same rule was
//! independently correct here). So the [`FileDiff`], [`FileOutline`] and
//! [`HunkShape`] types this module is retyped against are reached as
//! `crowbar_core::git::{FileDiff, FileOutline, HunkShape}` — a re-export
//! `crowbar-core` now carries specifically for this — rather than as a new
//! direct `crowbar-proto` dependency edge on this crate, which §4.2 does not
//! grant it. See `crowbar_core::git`'s module doc for that re-export's own
//! justification (same pattern as `crowbar_ui`'s `pub use gpui;`).
//!
//! # The `@pierre/diffs` retyping — where the shapes diverge, and what was assumed
//!
//! The TS source types this region against `@pierre/diffs`'s `Hunk` and
//! `FileDiffMetadata`. The brief's instruction was to re-type against
//! `crowbar-proto`'s own `Hunk` instead — **that does not work verbatim, and
//! saying so is the point of this section.** `crowbar_proto::domain_git::Hunk`
//! is `{ hunk_id, header, start_line, end_line }`: an index/reference into an
//! *already-parsed* `FileDiff.lines`, used to jump the reader to a specific
//! hunk by id. It carries no row-count or rendering-geometry fields at all —
//! it cannot represent "how tall does this hunk render," which is the entire
//! subject of `buildPlaceholderHunks`/`buildPlaceholderFileDiff`. Forcing the
//! algorithm's *output* through it would mean inventing fake `hunk_id`/
//! `header` values with no real meaning, exactly the kind of silent coercion
//! the brief warns against. So the output types here —
//! [`PlaceholderHunk`]/[`PlaceholderFileDiff`]/[`ChangeKind`] — are **new,
//! local to this crate**, sized from the fields `@pierre/diffs`'s `Hunk`/
//! `FileDiffMetadata` actually carried in this algorithm's own use of them
//! (every field read or written by the ported functions, below), not copied
//! from the library's full public surface.
//!
//! What DOES retype cleanly against `crowbar-proto`, without any new local
//! type: the region's two *input* shapes.
//! [`FileOutline`]/[`HunkShape`] (`review-window-api.ts`'s hand-written TS
//! types, not `@pierre/diffs`'s) already matched `crowbar_proto::domain_git`'s
//! generated types field-for-field before this item touched anything — reused
//! directly, no divergence. `GitDiff` (`git-types.ts`, the `files` parameter's
//! element type) maps onto `crowbar_proto::domain_git::FileDiff` almost as
//! cleanly, with one real, worth-stating divergence: TS's
//! `additions?`/`deletions?`/`uncommitted?` are optional (`number | undefined`
//! is a genuine third state distinct from a present zero or a present
//! negative sentinel), while `FileDiff`'s are non-optional `i64`/`bool` — a
//! `FileDiff`, once the daemon has produced one, always reports a value (the
//! same reasoning `crowbar_core::git::types::GitDiff`'s own module doc gives
//! for why *that* hand-rolled type keeps them `Option`-typed for a
//! *different* caller whose source data has no counts at all). **Assumption
//! made here:** this module's actual caller path (the branch-review summary
//! feeding `review-code-view.tsx`) always has counts, so the "absent" state
//! TS's `?` encodes is not reachable through this function in practice, and
//! [`count_of`]'s job shrinks correspondingly — it now only has to fold the
//! `-1` binary sentinel to `0`, not also fold `undefined`. If some future
//! caller reuses this module against a `FileDiff` genuinely built without
//! counts, it would report `0` where the TS source would have reported
//! `undefined`-derived `0` too (both fold to the same `count_of` output), so
//! this divergence is not observable in this algorithm's own output even in
//! that case — stated rather than left implicit.
//!
//! # What was **not** ported, and why
//!
//! - **`getImgSrc`** (`git-diff-helpers.ts`) — CONDITIONAL-live but
//!   presentation (a `data:` URI formatter), per the brief's explicit skip
//!   list.
//! - **`ReviewCodeView`/`ReviewCodeViewHandle`/`ReviewCodeViewProps`** — the
//!   React component and its React-facing types, per the brief's explicit
//!   skip list. GPUI does not port a React component 1:1.
//! - **`parseSingleFilePatch`'s actual patch-text parsing.** The function
//!   itself — a ~10-line wrapper the mapping doc's own §1 "Finding" already
//!   identifies as delegating entirely to `@pierre/diffs`'s `parsePatchFiles`
//!   — is real unified-diff-*text* parsing, not something this algorithm
//!   computes. Porting it verbatim would mean reimplementing (or literally
//!   depending on) `@pierre/diffs`, exactly what "do not pull `@pierre/diffs`
//!   semantics into a Rust crate" forbids. What genuinely *is* this
//!   function's own logic — try each already-parsed batch in order, take its
//!   first file, stop at the first hit, treat a failed parse as "no
//!   metadata" — is ported as [`first_parsed_file`], generic over an
//!   already-produced parse result rather than over a parser.
//!
//!   **Crate survey, done before assuming a hand-rolled parser was the right
//!   call (same discipline the `ignore`-crate-vs-cascade decision in
//!   `file-tree-gitignore.ts`'s port already set as precedent — general
//!   matching to a crate, Crowbar-specific policy hand-written):**
//!
//!   | crate | license | last release (as of 2026-08) | git-format support |
//!   |---|---|---|---|
//!   | [`patch`](https://crates.io/crates/patch) | MIT | 0.7.0, Dec 2022 — no releases since, ~4 years stale | "forgiving" of git's extra headers by *ignoring* them; does not surface renames, no-EOF-newline, or binary status as data |
//!   | [`gitpatch`](https://crates.io/crates/gitpatch) (a maintained fork of `patch`, same author-line API) | MIT | 0.7.1, Apr 2025 | same gap as `patch` — "forgiving" means tolerating the extra headers, not modelling them; still no structured rename/no-EOF-newline/binary output despite the name |
//!   | [`diffy`](https://crates.io/crates/diffy) | MIT OR Apache-2.0 | 0.5.1, Jul 2026 — actively maintained | its `patch_set` module, parsed with `ParseOptions::gitdiff()`, is purpose-built for this: `FileOperation` has explicit `Rename { from, to }` and `Copy { from, to }` variants populated *only* when git's extended headers say so, `FilePatch` exposes old/new file mode from those headers, and it is a streaming multi-file iterator (this daemon's patch responses are exactly that: one file per `/review/patch` call today, but the shape generalises) |
//!
//!   Zed was checked too, since spec §5.2 makes it this surface's reference.
//!   It turns out not to be a precedent here: Zed's `buffer_diff` crate
//!   computes a diff between a live buffer and a base-text blob it already
//!   holds (the same job `imara-diff`/`similar` do) — it never parses
//!   *serialised* patch text, because Zed always has both sides of the
//!   content in memory. Crowbar's daemon, by contrast, hands the client an
//!   already-`git diff`-rendered patch STRING (`GET /review/patch`), so this
//!   is a real parsing problem Zed's own architecture simply doesn't have.
//!
//!   **Recommendation for whoever builds the real (non-placeholder) parser:
//!   `diffy`'s `patch_set` module, not a hand-rolled parser and not
//!   `patch`/`gitpatch`.** Reasoning: `patch`/`gitpatch` are stale-to-
//!   maintained but structurally the same design, and that design's whole
//!   strategy is "ignore what git adds" — exactly the renames and
//!   no-newline markers `@pierre/diffs`'s `FileDiffMetadata.type` /
//!   `Hunk.noEOFCRDeletions`/`noEOFCRAdditions` need, so adopting either
//!   would still require re-scanning the raw text by hand for precisely the
//!   headers the crate discards, forfeiting most of the "let a crate do it"
//!   benefit. `diffy` surfaces the git-specific structure directly.
//!
//!   **One real semantic divergence a future integration must bridge, found
//!   by checking rather than assuming — the same class of gap the
//!   `ignore`-crate decision found for `.gitignore` (a trailing `/` in TS vs.
//!   a separate bool in the crate):** `@pierre/diffs`'s `Hunk` carries
//!   `noEOFCRDeletions`/`noEOFCRAdditions` as **explicit booleans**; `diffy`'s
//!   `Line` instead carries the terminating `\n` (or its absence) as part of
//!   each line's own text — "no newline at end of file" is the *fact* that
//!   the final `Delete`/`Insert` line's bytes don't end in `\n`, not a
//!   separate flag. A real integration derives the two booleans by checking
//!   that fact on the last removed/added line of each hunk; defaulting them
//!   to `false` without doing that check would silently misreport every
//!   no-trailing-newline file, the same class of bug a verbatim
//!   directory-vs-bool port would have been for `.gitignore`. `diffy` also
//!   has no `Copy` counterpart in `@pierre/diffs`'s `ChangeTypes` — Crowbar's
//!   daemon is not known to emit a bare copy-only diff for this surface, so
//!   this is flagged rather than resolved.
//!
//!   **Not implemented here.** Building the real `FileDiffMetadata`
//!   construction — tokenising hunk content into line structures, running
//!   `diffy::patch_set` end to end, deriving the two EOF-newline booleans —
//!   is separate, larger surface than this item's placeholder-*geometry*
//!   scope (estimating size *before* the real patch arrives), the same way
//!   `patch-window.ts`'s viewport scheduler and `diff-search.ts` were already
//!   carved out as separate items rather than folded in here. `diffy` is
//!   therefore **not** added as a `crowbar-diff` dependency by this item —
//!   doing so with no caller would be a manifest edit with nothing to justify
//!   it. This survey is left here so the item that does build the real parser
//!   does not have to re-run it.
//!
//!   `patchCacheKey` has no such issue and is ported in full as
//!   [`patch_cache_key`], with one stated divergence in its own doc comment
//!   (byte length vs. UTF-16 code unit length).
//!
//! # The real branches, enumerated before writing any Rust
//!
//! * **[`distribute_context`]:** empty `shapes` → `[]`. Every hunk's
//!   `min(old_lines, new_lines)` is `0` (pure-addition or pure-deletion hunks
//!   only) → every element `0`, regardless of `deletions`. `deletions == 0`
//!   → every element `0`, regardless of the caps. Otherwise: the total
//!   context to distribute is `old_total − deletions`, clamped to
//!   `[0, cap_total]` (never negative, never more than every hunk's combined
//!   capacity), then spread across hunks proportionally to each hunk's own
//!   cap and rounded *independently per hunk* — the per-hunk roundings are
//!   not corrected to sum exactly back to the total, matching the TS source,
//!   which does the same independent `Math.round` per element.
//! * **[`build_placeholder_hunks`]:** empty `shapes` → `[]` (the loop simply
//!   does not run). Per hunk, `shared` is the context estimate clamped to
//!   `min(old_lines, new_lines)` — never more context than the hunk
//!   physically has room for on either side. `collapsed_before` is the gap
//!   between this hunk's start and the *previous* hunk's end, floored at `0`
//!   (never negative even if geometry is inconsistent); the running
//!   `unified_line_start`/`split_line_start`/`*_line_index` counters
//!   accumulate across hunks in shape order, which is what makes a later
//!   hunk's position depend on every earlier one.
//! * **[`build_tail_hunk`]:** `hunks` empty (an outline with a partial flag
//!   but zero delivered hunks — e.g. a binary-then-reclassified file) → every
//!   running position defaults to `1`/`0` exactly as a from-scratch first
//!   hunk would; non-empty → every position continues from the *previous*
//!   hunk's end. **`hunk_specs` is `None`** on a tail hunk — the TS object
//!   literal omits the field entirely, unlike every hunk `build_placeholder_hunks`
//!   itself produces (always `Some`), which is why [`PlaceholderHunk::hunk_specs`]
//!   is `Option<String>` rather than a plain `String`.
//! * **[`reserve_at_most`]:** `unified_line_count <= room` → unchanged, no
//!   scaling needed. `room <= 0` → *also* unchanged — deliberately: scaling
//!   to zero-or-negative room would collapse the hunk to nothing, which is
//!   never the caller's intent (both call sites only reach a `room <= 0`
//!   hunk by NOT calling this function under that condition today —
//!   `build_placeholder_file_diff`'s tail-hunk call is itself gated on
//!   `room > 0`, and `trim_to_patch_cap`'s call always passes the cap itself,
//!   a positive constant — so this branch is a guard against a caller this
//!   module does not currently have, kept because the TS source keeps it).
//!   Otherwise: `unified_line_count` is capped to `room` exactly, and
//!   `split_line_count` is scaled by the same ratio and floored at `1` (never
//!   `0` — a hunk that reserves *any* space still reserves at least one
//!   row). Every other field, in particular `addition_lines`/`deletion_lines`
//!   (the ± counts the file header sums), is left untouched — this is the
//!   fix the TS source's own doc comment documents: scaling the ± counts too
//!   made a 420,000-line file announce itself as "+20000".
//! * **[`trim_to_patch_cap`]:** hunks are kept in order while the running
//!   `unified_line_count` total stays `<= patch_line_cap`; the FIRST hunk
//!   that would push the total over the cap, and every hunk after it, is
//!   dropped. If at least one hunk fit, that prefix is returned as-is. If
//!   NONE fit — the very first hunk alone already exceeds the cap, a
//!   whole-file rewrite as a single hunk — dropping leaves nothing to
//!   reserve space at all, so the first hunk is kept but scaled down via
//!   [`reserve_at_most`] instead of dropped. An empty `hunks` input returns
//!   empty (there is no first hunk to fall back to).
//! * **[`build_placeholder_file_diff`]:** the "top up from summary counts"
//!   block only runs when `outline` is present, `outline.is_partial` is
//!   true, AND there is `room` left under the cap after the already-trimmed
//!   hunks — three independent gates, all required. Within it, the missing
//!   additions/deletions are floored at `0` (never negative — the trimmed
//!   hunks may already account for more than the summary's counts in edge
//!   cases) and a tail hunk is appended ONLY if the combined missing amount
//!   is `> 0`; the tail hunk itself is then reserved to whatever `room`
//!   remains, never trimmed by [`trim_to_patch_cap`] (that already ran, and
//!   does not run again). `is_partial` on the OUTPUT is unconditionally
//!   `true` regardless of the input outline's own partial flag — a
//!   placeholder is always partial by construction, since it is standing in
//!   for content that has not arrived yet.

use std::collections::HashMap;

use crowbar_core::git::{FileDiff, FileOutline, HunkShape};

// ── File classification ─────────────────────────────────────────────

/// Mirrors `ReviewFileKind`: how a changed file is rendered.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ReviewFileKind {
    Diff,
    Image,
    Binary,
}

/// Mirrors `ReviewFileEntry`.
#[derive(Debug, Clone, PartialEq)]
pub struct ReviewFileEntry {
    pub path: String,
    pub kind: ReviewFileKind,
    /// The summary row — status flags and ± counts, never line content.
    pub file: FileDiff,
    /// Hunk geometry for the path, absent only if the outline disagrees with
    /// the summary.
    pub outline: Option<FileOutline>,
}

const IMAGE_EXTENSIONS: &[&str] = &[
    "apng", "avif", "bmp", "gif", "ico", "jpeg", "jpg", "png", "svg", "tif", "tiff", "webp",
];

fn is_image_path(path: &str) -> bool {
    match path.rfind('.') {
        Some(dot) => {
            let ext = path[dot + 1..].to_lowercase();
            IMAGE_EXTENSIONS.contains(&ext.as_str())
        }
        None => false,
    }
}

/// Mirrors `partitionReviewFiles`. Classifies every changed file, preserving
/// the summary's order.
///
/// Binary-ness comes from the OUTLINE when a matching one exists (git emits
/// no `@@` headers for a binary file, so the outline reports it directly),
/// falling back to the summary's own flag only when no outline entry matches
/// the path at all — not merely when the outline says `false`. Image-ness is
/// then decided by extension.
#[must_use]
pub fn partition_review_files(files: &[FileDiff], outline: &[FileOutline]) -> Vec<ReviewFileEntry> {
    let mut by_path: HashMap<&str, &FileOutline> = HashMap::new();
    for entry in outline {
        by_path.entry(entry.path.as_str()).or_insert(entry);
    }

    files
        .iter()
        .map(|file| {
            let path = file.file_path.clone();
            let shape = by_path.get(path.as_str()).copied();
            let binary = shape
                .map(|s| s.is_binary)
                .unwrap_or_else(|| file.is_binary.unwrap_or(false));

            let kind = if !binary {
                ReviewFileKind::Diff
            } else if file.is_image.unwrap_or_else(|| is_image_path(&path)) {
                ReviewFileKind::Image
            } else {
                ReviewFileKind::Binary
            };

            ReviewFileEntry {
                path,
                kind,
                file: file.clone(),
                outline: shape.cloned(),
            }
        })
        .collect()
}

// ── Placeholder geometry ────────────────────────────────────────────

/// Mirrors `ChangeTypes` as this algorithm actually produces it (`new` /
/// `deleted` / `rename-changed` / `change` — the four values
/// `changeTypeOf` returns). A local enum, not `@pierre/diffs`'s type: see
/// the module doc's retyping section.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum ChangeKind {
    New,
    Deleted,
    RenameChanged,
    Change,
}

/// One hunk's rendering-geometry estimate for a placeholder file diff — the
/// native counterpart to `@pierre/diffs`'s `Hunk`, restricted to exactly the
/// fields this module's algebra reads or writes. **Not**
/// `crowbar_proto::domain_git::Hunk`, a different concept entirely (an
/// index/reference into already-parsed line content) — see the module doc.
///
/// `hunkContent` (`@pierre/diffs`'s field for actual line text) is not
/// carried here: every hunk this module constructs sets it to an empty
/// array, unconditionally, and no test in the TS suite exercises it — a
/// field with no producer variation and no test is the same
/// "declaration, not behaviour" shape `crowbar_core::git::types`'s module
/// doc already avoids for an analogous case.
#[derive(Debug, Clone, PartialEq)]
pub struct PlaceholderHunk {
    pub collapsed_before: i64,
    pub addition_start: i64,
    pub addition_count: i64,
    pub addition_lines: i64,
    pub addition_line_index: i64,
    pub deletion_start: i64,
    pub deletion_count: i64,
    pub deletion_lines: i64,
    pub deletion_line_index: i64,
    /// `None` only for a synthetic tail hunk ([`build_tail_hunk`]) — every
    /// hunk built from real outline geometry ([`build_placeholder_hunks`])
    /// carries `Some`. See the module doc's branch enumeration.
    pub hunk_specs: Option<String>,
    pub split_line_start: i64,
    pub split_line_count: i64,
    pub unified_line_start: i64,
    pub unified_line_count: i64,
    pub no_eof_cr_deletions: bool,
    pub no_eof_cr_additions: bool,
}

/// The content-free placeholder a file occupies before its patch arrives —
/// native counterpart to `@pierre/diffs`'s `FileDiffMetadata`, restricted to
/// exactly the fields [`build_placeholder_file_diff`] sets. See the module
/// doc for the retyping rationale.
#[derive(Debug, Clone, PartialEq)]
pub struct PlaceholderFileDiff {
    pub name: String,
    pub prev_name: Option<String>,
    pub kind: ChangeKind,
    pub hunks: Vec<PlaceholderHunk>,
    pub split_line_count: i64,
    pub unified_line_count: i64,
    /// Always `true` on a placeholder — hardcoded, not derived from the
    /// input outline's own `is_partial`. See the module doc.
    pub is_partial: bool,
    /// Always empty by construction — a placeholder holds no line content.
    /// Kept as a field (rather than dropped, unlike [`PlaceholderHunk`]'s
    /// `hunk_content`) because the TS test suite explicitly asserts on it
    /// (`buildPlaceholderFileDiff`'s first ported test, below).
    pub deletion_lines: Vec<String>,
    pub addition_lines: Vec<String>,
}

/// A summary count, or `0` when it is the binary sentinel (`-1`) or merely
/// absent-in-spirit (`0`). TS's `countOf` also folds `undefined` here; see
/// the module doc's retyping section for why that state is not reachable
/// through a non-optional `i64`.
fn count_of(value: i64) -> i64 {
    if value > 0 { value } else { 0 }
}

fn sum_by<T>(items: &[T], f: impl Fn(&T) -> i64) -> i64 {
    items.iter().map(f).sum()
}

fn change_kind_of(file: &FileDiff) -> ChangeKind {
    if file.is_new {
        return ChangeKind::New;
    }
    if file.is_deleted {
        return ChangeKind::Deleted;
    }
    if file.is_renamed {
        return ChangeKind::RenameChanged;
    }
    ChangeKind::Change
}

/// `round(numerator / denominator)` for non-negative operands, matching JS's
/// `Math.round` (rounds half towards +Infinity) via exact integer
/// arithmetic — every call site in this module operates on non-negative
/// line counts, so `(2n + d) / (2d)` (integer division, which truncates
/// toward zero for non-negative operands, i.e. floors) is the same value
/// `Math.round(n / d)` would produce, without `crowbar-diff`'s clippy gate
/// having to reason about `i64 as f64` precision loss for a ratio that is
/// always exact here. `denominator == 0` returns `0` defensively; every
/// caller already guards against it, so this is a belt the arithmetic
/// itself does not need but the function signature should not silently
/// assume.
fn round_ratio(numerator: i64, denominator: i64) -> i64 {
    if denominator == 0 {
        return 0;
    }
    (2 * numerator + denominator) / (2 * denominator)
}

/// Mirrors `distributeContext`. See the module doc's branch enumeration.
fn distribute_context(shapes: &[HunkShape], deletions: i64) -> Vec<i64> {
    let caps: Vec<i64> = shapes.iter().map(|s| s.old_lines.min(s.new_lines)).collect();
    let cap_total: i64 = caps.iter().sum();
    if cap_total == 0 || deletions == 0 {
        return caps.iter().map(|_| 0).collect();
    }

    let old_total: i64 = shapes.iter().map(|s| s.old_lines).sum();
    let context_total = (old_total - deletions).max(0).min(cap_total);
    caps.iter()
        .map(|&cap| round_ratio(context_total * cap, cap_total))
        .collect()
}

/// Mirrors `buildPlaceholderHunks`. See the module doc's branch enumeration.
fn build_placeholder_hunks(shapes: &[HunkShape], context: &[i64]) -> Vec<PlaceholderHunk> {
    let mut hunks = Vec::with_capacity(shapes.len());
    let mut unified_line_start = 0i64;
    let mut split_line_start = 0i64;
    let mut addition_line_index = 0i64;
    let mut deletion_line_index = 0i64;
    let mut previous_old_end = 1i64;

    for (i, shape) in shapes.iter().enumerate() {
        let shared = context
            .get(i)
            .copied()
            .unwrap_or(0)
            .min(shape.old_lines)
            .min(shape.new_lines);
        let addition_lines = shape.new_lines - shared;
        let deletion_lines = shape.old_lines - shared;
        let unified_line_count = shared + addition_lines + deletion_lines;
        let split_line_count = shared + addition_lines.max(deletion_lines);

        hunks.push(PlaceholderHunk {
            collapsed_before: (shape.old_start - previous_old_end).max(0),
            addition_start: shape.new_start,
            addition_count: shape.new_lines,
            addition_lines,
            addition_line_index,
            deletion_start: shape.old_start,
            deletion_count: shape.old_lines,
            deletion_lines,
            deletion_line_index,
            hunk_specs: Some(format!(
                "@@ -{},{} +{},{} @@",
                shape.old_start, shape.old_lines, shape.new_start, shape.new_lines
            )),
            split_line_start,
            split_line_count,
            unified_line_start,
            unified_line_count,
            no_eof_cr_deletions: false,
            no_eof_cr_additions: false,
        });

        unified_line_start += unified_line_count;
        split_line_start += split_line_count;
        addition_line_index += shape.new_lines;
        deletion_line_index += shape.old_lines;
        previous_old_end = shape.old_start + shape.old_lines;
    }

    hunks
}

/// Mirrors `buildTailHunk`. See the module doc's branch enumeration,
/// including the empty-`hunks` default and the `hunk_specs: None` divergence
/// from every hunk `build_placeholder_hunks` itself produces.
fn build_tail_hunk(hunks: &[PlaceholderHunk], additions: i64, deletions: i64) -> PlaceholderHunk {
    let previous = hunks.last();
    PlaceholderHunk {
        collapsed_before: 0,
        addition_start: previous.map_or(1, |p| p.addition_start + p.addition_count),
        addition_count: additions,
        addition_lines: additions,
        addition_line_index: previous.map_or(0, |p| p.addition_line_index + p.addition_count),
        deletion_start: previous.map_or(1, |p| p.deletion_start + p.deletion_count),
        deletion_count: deletions,
        deletion_lines: deletions,
        deletion_line_index: previous.map_or(0, |p| p.deletion_line_index + p.deletion_count),
        hunk_specs: None,
        split_line_start: previous.map_or(0, |p| p.split_line_start + p.split_line_count),
        split_line_count: additions.max(deletions),
        unified_line_start: previous.map_or(0, |p| p.unified_line_start + p.unified_line_count),
        unified_line_count: additions + deletions,
        no_eof_cr_deletions: false,
        no_eof_cr_additions: false,
    }
}

/// Mirrors `reserveAtMost`. See the module doc's branch enumeration for why
/// `room <= 0` is a no-op rather than a collapse to zero.
fn reserve_at_most(hunk: PlaceholderHunk, room: i64) -> PlaceholderHunk {
    if hunk.unified_line_count <= room || room <= 0 {
        return hunk;
    }
    let split_line_count = round_ratio(hunk.split_line_count * room, hunk.unified_line_count).max(1);
    PlaceholderHunk {
        unified_line_count: room,
        split_line_count,
        ..hunk
    }
}

/// Mirrors `trimToPatchCap`. See the module doc's branch enumeration.
fn trim_to_patch_cap(hunks: &[PlaceholderHunk], patch_line_cap: i64) -> Vec<PlaceholderHunk> {
    let mut kept = Vec::new();
    let mut unified = 0i64;
    for hunk in hunks {
        if unified + hunk.unified_line_count > patch_line_cap {
            break;
        }
        kept.push(hunk.clone());
        unified += hunk.unified_line_count;
    }
    if !kept.is_empty() {
        return kept;
    }
    match hunks.first() {
        Some(first) => vec![reserve_at_most(first.clone(), patch_line_cap)],
        None => Vec::new(),
    }
}

/// Mirrors `buildPlaceholderFileDiff`.
///
/// `patch_line_cap` mirrors `patch-window.ts`'s `PATCH_LINE_CAP` constant
/// (`20_000` in the TS source). That module's 8 exports are explicitly out
/// of scope for this item (a separate, not-yet-landed port target, per the
/// brief) — rather than embed a second, potentially drifting copy of a
/// constant owned by a module this crate does not have yet, this function
/// takes it as a parameter. Callers pass `20_000` today; once
/// `patch-window.ts` lands in this crate, its constant becomes the one real
/// caller here.
///
/// See the module doc's branch enumeration for the three independent gates
/// on the "top up from summary counts" block.
#[must_use]
pub fn build_placeholder_file_diff(
    file: &FileDiff,
    outline: Option<&FileOutline>,
    patch_line_cap: i64,
) -> PlaceholderFileDiff {
    let shapes: &[HunkShape] = outline.map_or(&[], |o| o.hunks.as_slice());
    let additions = count_of(file.additions);
    let deletions = count_of(file.deletions);
    let context = distribute_context(shapes, deletions);
    let mut hunks = trim_to_patch_cap(&build_placeholder_hunks(shapes, &context), patch_line_cap);

    // A capped outline stops at the server's per-file hunk limit, so its
    // geometry is a LOWER bound on the file — sizing from it alone
    // under-reserves by however much the cap cut. Every changed line renders
    // one unified row, so the summary's ± counts are a floor the outline
    // must be topped up to.
    let room = patch_line_cap - sum_by(&hunks, |h| h.unified_line_count);
    if let Some(o) = outline {
        if o.is_partial && room > 0 {
            let missing_additions = (additions - sum_by(&hunks, |h| h.addition_lines)).max(0);
            let missing_deletions = (deletions - sum_by(&hunks, |h| h.deletion_lines)).max(0);
            if missing_additions + missing_deletions > 0 {
                let tail = build_tail_hunk(&hunks, missing_additions, missing_deletions);
                hunks.push(reserve_at_most(tail, room));
            }
        }
    }

    let name = outline.map_or_else(|| file.file_path.clone(), |o| o.path.clone());
    let prev_name_candidate = outline
        .and_then(|o| o.old_path.clone())
        .or_else(|| file.old_path.clone());
    let prev_name = prev_name_candidate.filter(|p| *p != name);

    PlaceholderFileDiff {
        split_line_count: sum_by(&hunks, |h| h.split_line_count),
        unified_line_count: sum_by(&hunks, |h| h.unified_line_count),
        name,
        prev_name,
        kind: change_kind_of(file),
        hunks,
        is_partial: true,
        deletion_lines: Vec::new(),
        addition_lines: Vec::new(),
    }
}

// ── Patch identity ──────────────────────────────────────────────────

/// Mirrors `patchCacheKey`.
///
/// **One stated divergence:** JS's `patch.length` counts UTF-16 code units;
/// Rust's `str::len()` counts UTF-8 bytes. For ASCII-only unified-diff text
/// (the overwhelming case — source code and diff punctuation are almost
/// always ASCII) the two agree exactly. For a patch containing multi-byte
/// UTF-8 content the two would produce different numeric suffixes for the
/// same logical patch — but the key's only job is to change whenever the
/// CONTENT does, within one process's own comparisons (see the TS source's
/// own doc comment: keying `wsId:path` alone broke re-fetch-after-expand
/// because two different-length patches shared a key). A byte-length and a
/// UTF-16-length both satisfy that "changes when content changes" property
/// on their own terms; they are simply never compared against each other, so
/// this divergence changes what the key's digits look like, never whether
/// caching behaves correctly.
#[must_use]
pub fn patch_cache_key(ws_id: &str, commit: Option<&str>, path: &str, patch: &str) -> String {
    format!("{ws_id}:{}:{path}:{}", commit.unwrap_or(""), patch.len())
}

/// The wrapper-only half of `parseSingleFilePatch` — see the module doc's
/// "What was not ported" section for why the actual patch-text parsing
/// (`@pierre/diffs`'s `parsePatchFiles`) is not reimplemented here.
///
/// Takes an already-produced parse result: `None` for a failed parse
/// (mirrors the TS `try`/`catch` swallowing any parser error into
/// `undefined`), `Some(batches)` otherwise, where each batch is that parse
/// attempt's own `files` list. Returns the first present file at index `0`
/// of the first batch whose index `0` is non-empty — mirrors
/// `for (const parsed of parsePatchFiles(...)) { const file = parsed.files[0]; if (file != null) return file }`
/// exactly: only index `0` of each batch is ever consulted, not every file
/// in every batch.
#[must_use]
pub fn first_parsed_file(
    parsed: Option<Vec<Vec<PlaceholderFileDiff>>>,
) -> Option<PlaceholderFileDiff> {
    let batches = parsed?;
    for batch in batches {
        if let Some(file) = batch.into_iter().next() {
            return Some(file);
        }
    }
    None
}

#[cfg(test)]
mod tests {
    use super::{
        ChangeKind, FileDiff, FileOutline, HunkShape, ReviewFileKind, build_placeholder_file_diff,
        build_placeholder_hunks, build_tail_hunk, count_of, distribute_context, first_parsed_file,
        partition_review_files, patch_cache_key, reserve_at_most, round_ratio, trim_to_patch_cap,
    };

    const PATCH_LINE_CAP: i64 = 20_000;

    fn path_of(i: usize) -> String {
        format!("src/pkg/file{i}.ts")
    }

    fn text_file(i: usize) -> FileDiff {
        FileDiff {
            file_path: path_of(i),
            old_path: None,
            new_path: None,
            is_new: false,
            is_deleted: false,
            is_renamed: false,
            is_binary: None,
            is_image: None,
            old_blob_base64: None,
            new_blob_base64: None,
            lines: Vec::new(),
            additions: 1,
            deletions: 1,
            hunks: Vec::new(),
            uncommitted: false,
        }
    }

    fn hunk_shape(old_start: i64, old_lines: i64, new_start: i64, new_lines: i64) -> HunkShape {
        HunkShape {
            old_start,
            old_lines,
            new_start,
            new_lines,
        }
    }

    fn text_outline(i: usize) -> FileOutline {
        FileOutline {
            path: path_of(i),
            old_path: None,
            hunks: vec![hunk_shape(1, 3, 1, 3)],
            is_partial: false,
            is_binary: false,
        }
    }

    fn many_files(count: usize) -> (Vec<FileDiff>, Vec<FileOutline>) {
        (
            (0..count).map(text_file).collect(),
            (0..count).map(text_outline).collect(),
        )
    }

    // === partitionReviewFiles ===
    // --- ported from web/src/__tests__/features/git/components/diff/review-code-view.test.tsx ---

    #[test]
    fn partition_produces_exactly_one_entry_per_file_in_summary_order() {
        let (files, outline) = many_files(5);
        let entries = partition_review_files(&files, &outline);

        assert_eq!(entries.len(), files.len());
        assert_eq!(
            entries.iter().map(|e| e.path.clone()).collect::<Vec<_>>(),
            files.iter().map(|f| f.file_path.clone()).collect::<Vec<_>>()
        );
        assert!(entries.iter().all(|e| e.kind == ReviewFileKind::Diff));
    }

    #[test]
    fn partition_routes_binaries_away_from_diff_images_separately_from_other_binaries() {
        let files = vec![
            FileDiff {
                file_path: "assets/logo.png".to_string(),
                ..text_file(0)
            },
            FileDiff {
                file_path: "assets/blob.bin".to_string(),
                ..text_file(1)
            },
            text_file(2),
        ];
        let outline = vec![
            FileOutline {
                path: "assets/logo.png".to_string(),
                old_path: None,
                hunks: Vec::new(),
                is_partial: false,
                is_binary: true,
            },
            FileOutline {
                path: "assets/blob.bin".to_string(),
                old_path: None,
                hunks: Vec::new(),
                is_partial: false,
                is_binary: true,
            },
            text_outline(2),
        ];

        let kinds: Vec<ReviewFileKind> = partition_review_files(&files, &outline)
            .into_iter()
            .map(|e| e.kind)
            .collect();
        assert_eq!(
            kinds,
            vec![
                ReviewFileKind::Image,
                ReviewFileKind::Binary,
                ReviewFileKind::Diff
            ]
        );
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn partition_falls_back_to_the_file_own_binary_flag_when_no_outline_entry_matches() {
        // No outline row at all for this path (not merely `isBinary: false`)
        // — the fallback to `file.is_binary` this module doc's branch note
        // calls out.
        let file = FileDiff {
            is_binary: Some(true),
            ..text_file(0)
        };
        let entries = partition_review_files(&[file], &[]);
        assert_eq!(entries[0].kind, ReviewFileKind::Binary);
    }

    #[test]
    fn partition_prefers_the_outline_binary_flag_even_when_it_says_false() {
        // The outline exists and says non-binary; the file summary's own
        // (stale, or simply absent) flag must not override it.
        let file = FileDiff {
            is_binary: Some(true),
            ..text_file(0)
        };
        let outline = vec![text_outline(0)]; // is_binary: false
        let entries = partition_review_files(&[file], &outline);
        assert_eq!(entries[0].kind, ReviewFileKind::Diff);
    }

    // === buildPlaceholderFileDiff ===
    // --- ported from web/src/__tests__/features/git/components/diff/review-code-view.test.tsx ---

    #[test]
    fn placeholder_reserves_geometry_from_hunk_shapes_alone_holding_no_line_content() {
        let file_diff =
            build_placeholder_file_diff(&text_file(0), Some(&text_outline(0)), PATCH_LINE_CAP);

        assert_eq!(file_diff.addition_lines, Vec::<String>::new());
        assert_eq!(file_diff.deletion_lines, Vec::<String>::new());
        // @@ -1,3 +1,3 @@ with ±1 is 2 context rows + 1 removal + 1 addition.
        assert_eq!(file_diff.unified_line_count, 4);
        assert_eq!(file_diff.split_line_count, 3);
    }

    #[test]
    fn placeholder_tops_a_partial_file_up_from_summary_counts_since_outline_is_a_lower_bound() {
        let file = FileDiff {
            additions: 40_000,
            deletions: 20_000,
            ..text_file(0)
        };
        let capped = FileOutline {
            is_partial: true,
            ..text_outline(0)
        };
        let complete = FileOutline {
            is_partial: false,
            ..text_outline(0)
        };

        let partial = build_placeholder_file_diff(&file, Some(&capped), PATCH_LINE_CAP);
        let complete = build_placeholder_file_diff(&file, Some(&complete), PATCH_LINE_CAP);

        assert!(partial.unified_line_count > complete.unified_line_count);
        // ...but never beyond what the capped patch will actually deliver.
        assert!(partial.unified_line_count <= PATCH_LINE_CAP);
    }

    #[test]
    fn placeholder_reserves_capped_height_but_still_reports_true_counts() {
        let monster = FileDiff {
            additions: 420_000,
            deletions: 0,
            ..text_file(0)
        };
        let one_huge_hunk = FileOutline {
            hunks: vec![hunk_shape(0, 0, 1, 420_000)],
            ..text_outline(0)
        };

        let placeholder =
            build_placeholder_file_diff(&monster, Some(&one_huge_hunk), PATCH_LINE_CAP);

        assert!(placeholder.unified_line_count <= PATCH_LINE_CAP);
        let reported: i64 = placeholder.hunks.iter().map(|h| h.addition_lines).sum();
        assert_eq!(reported, 420_000);
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn placeholder_with_no_outline_falls_back_to_the_file_own_path_and_zero_geometry() {
        let file = FileDiff {
            file_path: "orphan.ts".to_string(),
            old_path: Some("orphan-old.ts".to_string()),
            ..text_file(0)
        };
        let placeholder = build_placeholder_file_diff(&file, None, PATCH_LINE_CAP);
        assert_eq!(placeholder.name, "orphan.ts");
        assert_eq!(placeholder.prev_name.as_deref(), Some("orphan-old.ts"));
        assert!(placeholder.hunks.is_empty());
        assert_eq!(placeholder.unified_line_count, 0);
        assert!(placeholder.is_partial);
    }

    #[test]
    fn placeholder_never_reports_a_prev_name_equal_to_the_current_name() {
        let file = FileDiff {
            old_path: Some("same.ts".to_string()),
            ..text_file(0)
        };
        let outline = FileOutline {
            path: "same.ts".to_string(),
            old_path: Some("same.ts".to_string()),
            ..text_outline(0)
        };
        let placeholder = build_placeholder_file_diff(&file, Some(&outline), PATCH_LINE_CAP);
        assert_eq!(placeholder.name, "same.ts");
        assert_eq!(placeholder.prev_name, None);
    }

    #[test]
    fn placeholder_change_kind_precedence_matches_changetypeof() {
        let new = build_placeholder_file_diff(
            &FileDiff {
                is_new: true,
                ..text_file(0)
            },
            None,
            PATCH_LINE_CAP,
        );
        let deleted = build_placeholder_file_diff(
            &FileDiff {
                is_deleted: true,
                ..text_file(0)
            },
            None,
            PATCH_LINE_CAP,
        );
        let renamed = build_placeholder_file_diff(
            &FileDiff {
                is_renamed: true,
                ..text_file(0)
            },
            None,
            PATCH_LINE_CAP,
        );
        let changed = build_placeholder_file_diff(&text_file(0), None, PATCH_LINE_CAP);
        // is_new wins over every other flag, matching the TS if-chain order.
        let contradictory = build_placeholder_file_diff(
            &FileDiff {
                is_new: true,
                is_deleted: true,
                is_renamed: true,
                ..text_file(0)
            },
            None,
            PATCH_LINE_CAP,
        );

        assert_eq!(new.kind, ChangeKind::New);
        assert_eq!(deleted.kind, ChangeKind::Deleted);
        assert_eq!(renamed.kind, ChangeKind::RenameChanged);
        assert_eq!(changed.kind, ChangeKind::Change);
        assert_eq!(contradictory.kind, ChangeKind::New);
    }

    // === distributeContext — the substantive algorithm, exercised at unit level ===
    // --- new: not exercised by the TS suite directly (only through buildPlaceholderFileDiff) ---

    #[test]
    fn distribute_context_is_all_zero_for_empty_shapes() {
        assert_eq!(distribute_context(&[], 10), Vec::<i64>::new());
    }

    #[test]
    fn distribute_context_is_all_zero_when_every_hunk_cap_is_zero() {
        // Pure-addition hunks: old_lines is 0 on every one, so min(old,new) is
        // 0 regardless of deletions being non-zero.
        let shapes = vec![hunk_shape(1, 0, 1, 5), hunk_shape(10, 0, 10, 3)];
        assert_eq!(distribute_context(&shapes, 8), vec![0, 0]);
    }

    #[test]
    fn distribute_context_is_all_zero_when_deletions_is_zero_even_with_room() {
        let shapes = vec![hunk_shape(1, 5, 1, 5)];
        assert_eq!(distribute_context(&shapes, 0), vec![0]);
    }

    #[test]
    fn distribute_context_splits_proportionally_to_each_hunk_own_cap() {
        // Two equal-cap hunks (cap 4 each, total 8), old_total 8, deletions 4
        // → context_total = min(max(8-4,0),8) = 4, split evenly 2/2.
        let shapes = vec![hunk_shape(1, 4, 1, 4), hunk_shape(10, 4, 10, 4)];
        assert_eq!(distribute_context(&shapes, 4), vec![2, 2]);
    }

    #[test]
    fn distribute_context_clamps_the_total_to_the_combined_cap_not_beyond_it() {
        // Two hunks, cap 5 each (combined cap_total 10); old_total 105,
        // deletions 1 → naive old_total - deletions = 104, but cap_total
        // (10) must win the min().
        let shapes = vec![hunk_shape(1, 100, 1, 5), hunk_shape(200, 5, 200, 5)];
        let out = distribute_context(&shapes, 1);
        let total: i64 = out.iter().sum();
        assert!(total <= 10);
    }

    #[test]
    fn distribute_context_rounds_each_hunk_independently_half_up() {
        // cap_total 3 (caps 1,1,1), old_total 3, deletions 1 →
        // context_total = 2. Per hunk: round(2*1/3) = round(0.666..) = 1,
        // for all three — sum (3) legitimately EXCEEDS context_total (2),
        // which is the documented "not corrected to sum exactly" behaviour.
        let shapes = vec![
            hunk_shape(1, 1, 1, 1),
            hunk_shape(2, 1, 2, 1),
            hunk_shape(3, 1, 3, 1),
        ];
        let out = distribute_context(&shapes, 1);
        assert_eq!(out, vec![1, 1, 1]);
    }

    #[test]
    fn round_ratio_matches_js_math_round_half_up_at_the_half_boundary() {
        // JS Math.round(2.5) === 3 (rounds half towards +Infinity).
        assert_eq!(round_ratio(5, 2), 3);
        // Math.round(1.5) === 2.
        assert_eq!(round_ratio(3, 2), 2);
        // Math.round(0.5) === 1.
        assert_eq!(round_ratio(1, 2), 1);
    }

    // === buildPlaceholderHunks — the other substantive algorithm ===
    // --- new: not exercised by the TS suite directly ---

    #[test]
    fn build_hunks_is_empty_for_empty_shapes() {
        assert!(build_placeholder_hunks(&[], &[]).is_empty());
    }

    #[test]
    fn build_hunks_clamps_shared_context_to_the_hunk_own_side_minimums() {
        // context estimate (100) far exceeds either side (3/3) — shared must
        // clamp to 3, not consume more than the hunk has.
        let shapes = vec![hunk_shape(1, 3, 1, 3)];
        let hunks = build_placeholder_hunks(&shapes, &[100]);
        assert_eq!(hunks[0].addition_lines, 0);
        assert_eq!(hunks[0].deletion_lines, 0);
        assert_eq!(hunks[0].unified_line_count, 3);
        assert_eq!(hunks[0].split_line_count, 3);
    }

    #[test]
    fn build_hunks_clamps_shared_by_the_smaller_side_when_old_and_new_differ() {
        // old_lines (5) > new_lines (2): the min(old,new) clamp must bind on
        // new_lines specifically, not merely on old_lines — a hunk this
        // lopsided is exactly what a same-sided-only clamp would miss, since
        // min(context, old_lines) alone would let shared exceed new_lines
        // and drive addition_lines negative.
        let shapes = vec![hunk_shape(1, 5, 1, 2)];
        let hunks = build_placeholder_hunks(&shapes, &[10]);
        assert_eq!(hunks[0].addition_lines, 0);
        assert_eq!(hunks[0].deletion_lines, 3);
    }

    #[test]
    fn build_hunks_computes_collapsed_before_from_the_gap_to_the_previous_hunk() {
        let shapes = vec![hunk_shape(1, 3, 1, 3), hunk_shape(10, 2, 10, 2)];
        let hunks = build_placeholder_hunks(&shapes, &[0, 0]);
        // previous_old_end after hunk 0 is 1+3=4; hunk 1 starts at 10 → gap 6.
        assert_eq!(hunks[1].collapsed_before, 6);
    }

    #[test]
    fn build_hunks_never_reports_a_negative_collapsed_before_for_overlapping_geometry() {
        let shapes = vec![hunk_shape(10, 5, 10, 5), hunk_shape(5, 2, 5, 2)];
        let hunks = build_placeholder_hunks(&shapes, &[0, 0]);
        assert_eq!(hunks[1].collapsed_before, 0);
    }

    #[test]
    fn build_hunks_accumulates_running_positions_across_hunks() {
        let shapes = vec![hunk_shape(1, 4, 1, 2), hunk_shape(10, 1, 10, 1)];
        // hunk 0: shared = min(0,4,2) = 0, addition_lines = 2, deletion_lines
        // = 4, unified = 6, split = 4.
        let hunks = build_placeholder_hunks(&shapes, &[0, 0]);
        assert_eq!(hunks[0].unified_line_start, 0);
        assert_eq!(hunks[1].unified_line_start, 6);
        assert_eq!(hunks[0].split_line_start, 0);
        assert_eq!(hunks[1].split_line_start, 4);
        assert_eq!(hunks[0].addition_line_index, 0);
        assert_eq!(hunks[1].addition_line_index, 2);
        assert_eq!(hunks[0].deletion_line_index, 0);
        assert_eq!(hunks[1].deletion_line_index, 4);
    }

    #[test]
    fn build_hunks_sets_hunk_specs_from_the_shape_numbers() {
        let shapes = vec![hunk_shape(5, 3, 7, 2)];
        let hunks = build_placeholder_hunks(&shapes, &[0]);
        assert_eq!(hunks[0].hunk_specs.as_deref(), Some("@@ -5,3 +7,2 @@"));
    }

    // === buildTailHunk ===
    // --- new: not exercised by the TS suite directly ---

    #[test]
    fn tail_hunk_defaults_positions_when_no_prior_hunk_exists() {
        let tail = build_tail_hunk(&[], 10, 4);
        assert_eq!(tail.addition_start, 1);
        assert_eq!(tail.deletion_start, 1);
        assert_eq!(tail.addition_line_index, 0);
        assert_eq!(tail.deletion_line_index, 0);
        assert_eq!(tail.split_line_start, 0);
        assert_eq!(tail.unified_line_start, 0);
        assert_eq!(tail.hunk_specs, None);
        assert_eq!(tail.unified_line_count, 14);
        assert_eq!(tail.split_line_count, 10);
    }

    #[test]
    fn tail_hunk_continues_from_the_previous_hunk_end() {
        let shapes = vec![hunk_shape(1, 3, 1, 3)];
        let hunks = build_placeholder_hunks(&shapes, &[0]);
        let tail = build_tail_hunk(&hunks, 5, 2);
        let prev = &hunks[0];
        assert_eq!(tail.addition_start, prev.addition_start + prev.addition_count);
        assert_eq!(tail.deletion_start, prev.deletion_start + prev.deletion_count);
        assert_eq!(
            tail.unified_line_start,
            prev.unified_line_start + prev.unified_line_count
        );
        assert_eq!(tail.hunk_specs, None);
    }

    // === reserveAtMost ===
    // --- new: not exercised by the TS suite directly ---

    #[test]
    fn reserve_at_most_leaves_a_hunk_under_room_unchanged() {
        let hunks = build_placeholder_hunks(&[hunk_shape(1, 3, 1, 3)], &[0]);
        let hunk = hunks[0].clone();
        let reserved = reserve_at_most(hunk.clone(), 1000);
        assert_eq!(reserved, hunk);
    }

    #[test]
    fn reserve_at_most_leaves_a_hunk_unchanged_when_room_is_non_positive() {
        let hunks = build_placeholder_hunks(&[hunk_shape(1, 3, 1, 3)], &[0]);
        let hunk = hunks[0].clone();
        assert_eq!(reserve_at_most(hunk.clone(), 0), hunk);
        assert_eq!(reserve_at_most(hunk.clone(), -5), hunk);
    }

    #[test]
    fn reserve_at_most_scales_split_line_count_proportionally_and_caps_unified() {
        let shapes = vec![hunk_shape(1, 0, 1, 100)]; // shared 0, addition_lines 100
        let hunks = build_placeholder_hunks(&shapes, &[0]);
        let hunk = hunks[0].clone();
        assert_eq!(hunk.unified_line_count, 100);
        assert_eq!(hunk.split_line_count, 100);

        let reserved = reserve_at_most(hunk, 10);
        assert_eq!(reserved.unified_line_count, 10);
        assert_eq!(reserved.split_line_count, 10);
        // The ± counts are NOT scaled — this is the fix the TS source's own
        // doc comment documents (scaling them too misreported a 420,000-line
        // file as "+20000").
        assert_eq!(reserved.addition_lines, 100);
    }

    #[test]
    fn reserve_at_most_floors_split_line_count_at_one_never_zero() {
        let shapes = vec![hunk_shape(1, 0, 1, 1000)];
        let hunks = build_placeholder_hunks(&shapes, &[0]);
        let reserved = reserve_at_most(hunks[0].clone(), 1);
        assert_eq!(reserved.unified_line_count, 1);
        assert!(reserved.split_line_count >= 1);
    }

    // === trimToPatchCap ===
    // --- new: not exercised by the TS suite directly ---

    #[test]
    fn trim_keeps_every_hunk_that_fits_under_the_cap() {
        let shapes = vec![hunk_shape(1, 0, 1, 10), hunk_shape(20, 0, 20, 10)];
        let hunks = build_placeholder_hunks(&shapes, &[0, 0]);
        let trimmed = trim_to_patch_cap(&hunks, 100);
        assert_eq!(trimmed.len(), 2);
    }

    #[test]
    fn trim_keeps_a_hunk_that_lands_exactly_on_the_cap() {
        // Two 10-row hunks against a cap of exactly 20: the running total
        // after both is 20, never strictly OVER the cap, so both must be
        // kept — "exceed" is `>`, not `>=`.
        let shapes = vec![hunk_shape(1, 0, 1, 10), hunk_shape(20, 0, 20, 10)];
        let hunks = build_placeholder_hunks(&shapes, &[0, 0]);
        let trimmed = trim_to_patch_cap(&hunks, 20);
        assert_eq!(trimmed.len(), 2);
    }

    #[test]
    fn trim_drops_hunks_at_the_point_the_running_total_would_exceed_the_cap() {
        let shapes = vec![
            hunk_shape(1, 0, 1, 10),
            hunk_shape(20, 0, 20, 10),
            hunk_shape(40, 0, 40, 10),
        ];
        let hunks = build_placeholder_hunks(&shapes, &[0, 0, 0]);
        // Each hunk is 10 unified rows; cap 15 keeps only the first.
        let trimmed = trim_to_patch_cap(&hunks, 15);
        assert_eq!(trimmed.len(), 1);
    }

    #[test]
    fn trim_scales_the_first_hunk_down_when_it_alone_exceeds_the_cap() {
        let shapes = vec![hunk_shape(1, 0, 1, 1000)];
        let hunks = build_placeholder_hunks(&shapes, &[0]);
        let trimmed = trim_to_patch_cap(&hunks, 100);
        assert_eq!(trimmed.len(), 1);
        assert_eq!(trimmed[0].unified_line_count, 100);
    }

    #[test]
    fn trim_of_empty_hunks_is_empty() {
        assert!(trim_to_patch_cap(&[], 100).is_empty());
    }

    // === count_of, patch_cache_key, first_parsed_file ===
    // --- new: not exercised by the TS suite directly ---

    #[test]
    fn count_of_folds_the_binary_sentinel_and_zero_to_zero() {
        assert_eq!(count_of(-1), 0);
        assert_eq!(count_of(0), 0);
        assert_eq!(count_of(5), 5);
    }

    #[test]
    fn patch_cache_key_changes_with_patch_length_not_just_path() {
        let short = patch_cache_key("ws-1", None, "a.ts", "x");
        let long = patch_cache_key("ws-1", None, "a.ts", "xxxxxxxxxx");
        assert_ne!(short, long);
    }

    #[test]
    fn patch_cache_key_defaults_a_missing_commit_to_empty() {
        let key = patch_cache_key("ws-1", None, "a.ts", "patch");
        assert_eq!(key, "ws-1::a.ts:5");
    }

    #[test]
    fn first_parsed_file_returns_none_on_a_failed_parse() {
        assert_eq!(first_parsed_file(None), None);
    }

    #[test]
    fn first_parsed_file_skips_empty_batches_and_returns_the_first_present_file() {
        let placeholder = build_placeholder_file_diff(&text_file(0), None, PATCH_LINE_CAP);
        let batches = vec![vec![], vec![placeholder.clone()]];
        assert_eq!(first_parsed_file(Some(batches)), Some(placeholder));
    }

    #[test]
    fn first_parsed_file_only_consults_index_zero_of_each_batch() {
        let first = build_placeholder_file_diff(&text_file(0), None, PATCH_LINE_CAP);
        let second = build_placeholder_file_diff(&text_file(1), None, PATCH_LINE_CAP);
        let batches = vec![vec![first.clone(), second]];
        assert_eq!(first_parsed_file(Some(batches)), Some(first));
    }
}
