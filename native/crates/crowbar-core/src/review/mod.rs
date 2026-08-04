//! Review-thread model — spec §4.2's `"review-thread model"` bucket of
//! `crowbar-core`, the sixth Tier A area to land after [`super::workspace`]
//! (P3.53), [`super::git`] (P3.67), [`super::keymap`], [`super::settings`],
//! and [`super::file_tree`].
//!
//! Ported from the four non-component files
//! `native/mapping/tier-a-denominator.md` §7 ("Review threads") audits — the
//! one area of the three §7 covers where the export-liveness pass "found
//! nothing to prune" (§7's own words: every one of the area's 900 TS lines
//! is LIVE or CONDITIONAL, zero DEAD, zero TEST-ONLY):
//!
//! | module | ported from | what |
//! |---|---|---|
//! | [`state`] | `features/workspace/stores/slices/branch-review-slice.ts` | `ReviewMessage`/`ReviewThread`/`ReviewConversation` + `createBranchReviewSlice`'s state and mutators, as free functions |
//! | [`api`] | `features/git/api/review-api.ts` | `mapThread`+`mapReply`, `getReview`+`mapConversation` (its pure half — see that module's doc) |
//! | [`annotation`] | `features/git/components/diff/use-review-annotations.tsx` | the pure-helper half: the old/new <-> deletions/additions side map, wrap/unwrap, per-file grouping and counting |
//!
//! `features/git/components/review-thread-item.tsx` (the fourth file §7
//! counts, 424 lines) is a component — audited by the survey for
//! completeness and excluded from its own line total by its own convention;
//! not ported here for the same reason (Phase-4/GPUI presentation).
//!
//! # What this item did not port, and why
//!
//! * **`review-api.ts`'s other 16 exports** — `listThreads`, 9 CRUD/mutation
//!   transport functions, 6 supporting DTO types. Transport, not model — see
//!   [`api`]'s module doc, which also covers why `getReview` itself only
//!   ports its pure reshaping half.
//! * **`use-review-annotations.tsx`'s `useReviewAnnotations` hook** —
//!   store subscription, `useState`, submit/cancel handlers, JSX renderers.
//!   Phase-4/presentation; see [`annotation`]'s module doc for the one named
//!   type (`ReviewAnnotationLayer`) checked against that exclusion and found
//!   not to survive it as a literal struct.
//! * **`branch-review-slice.ts`'s `resolveReviewThread`** — `@deprecated` in
//!   its own doc comment, zero non-test callers, functionally identical to
//!   `set_review_thread_resolved(state, id, true)`. See [`state`]'s module
//!   doc for the full reasoning.
//! * **`ReviewState.diff` / `WireBranchReview.diff` (`MultiFileDiff`)** — no
//!   producer on the real backend, no consumer in the real frontend. See
//!   [`api`]'s module doc for both independent confirmations.
//!
//! # `@pierre/diffs`'s `DiffLineAnnotation<T>`
//!
//! Retyped natively in [`annotation`] rather than pulled in as a dependency —
//! `crowbar-core` cannot depend on a UI-facing diff-rendering package at all,
//! and the concept this port needs (a thread pinned to a side/line of a
//! file, with arbitrary metadata) is a handful of fields, not a rendering
//! library. See that module's doc for exactly what was assumed about the
//! type's shape and why.

pub mod annotation;
pub mod api;
pub mod state;

pub use annotation::{
    AnnotationSide, DRAFT_THREAD_ID, DiffLineAnnotation, ReviewAnnotation, annotation_to_thread,
    count_threads_by_path, group_annotations_by_path, is_draft_thread, thread_to_annotation,
    to_annotation_side, to_thread_side,
};
pub use api::{ReviewState, WireBranchChat, WireBranchReview, build_review_state, map_thread};
pub use state::{
    BranchReviewState, DiffStatus, ReviewConversation, ReviewMessage, ReviewThread, ThreadSide,
    add_review_conversation, add_review_message, add_review_thread, remove_review_thread,
    reveal_file, set_branch_review_conversations, set_branch_review_description,
    set_branch_review_diff_status, set_branch_review_merge_strategy, set_review_thread_resolved,
    upsert_review_thread,
};
