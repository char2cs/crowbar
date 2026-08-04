//! Review threads as diff line annotations — the pure half.
//!
//! Ported from `web/src/features/git/components/diff/use-review-annotations.tsx`
//! (395 lines), **only** the pure-helper half `native/mapping/tier-a-denominator.md`
//! §7 names: `isDraftThread`, `toAnnotationSide`, `toThreadSide`,
//! `threadToAnnotation`, `annotationToThread`, `groupAnnotationsByPath`,
//! `countThreadsByPath`, plus the types `ReviewAnnotation`, `DRAFT_THREAD_ID`.
//! The file's 11th export, the `useReviewAnnotations` hook itself — store
//! subscription, `useState` draft/error, submit/cancel handlers that call the
//! (not-ported-here) transport functions, and JSX renderers — is Phase-4/
//! presentation and is **not** ported, per the brief.
//!
//! # `DiffLineAnnotation<T>`: the known `@pierre/diffs` entanglement, retyped natively
//!
//! TS `ReviewAnnotation = DiffLineAnnotation<ReviewThread>` reuses a generic
//! type from `@pierre/diffs`, the same library-type entanglement already
//! flagged for the diff-algebra area's placeholder-hunk geometry. This port
//! does not depend on that library (nor could it — `crowbar-core` cannot
//! depend on a UI package at all); [`DiffLineAnnotation`] below is a native
//! reimplementation of exactly the shape this file's code actually needs.
//!
//! **What was assumed, since the library's own `.d.ts` is not available to
//! read directly (no `node_modules` in this tree):** every construction site
//! of a `ReviewAnnotation` in the ported TS source and its test suite —
//! `threadToAnnotation`'s object literal, and the one place a test builds a
//! `ReviewAnnotation` literal directly rather than through that function
//! (`use-review-annotations.test.tsx`, the round-trip test) — uses exactly
//! three fields: `side`, `lineNumber`, `metadata`. No other field is ever
//! read or written anywhere in the ported code. [`DiffLineAnnotation<T>`]
//! below carries exactly those three and no more; if the real library type
//! carries additional fields with library-supplied defaults, this port does
//! not need or model them, because nothing in the ported logic ever touches
//! them.
//!
//! # `ReviewAnnotationLayer` — named in the brief, not ported as a struct
//!
//! The brief's scope list includes `ReviewAnnotationLayer` alongside the
//! seven pure functions. Checked against the source rather than ported
//! as-is: TS `ReviewAnnotationLayer` is `useReviewAnnotations`'s own return
//! type, declared in the same file and imported nowhere else in `web/src`
//! (`git grep -n ReviewAnnotationLayer web/src` turns up only its
//! declaration and the hook's signature). Of its 11 fields, 2
//! (`annotationsByPath`, `threadCounts`) are exactly the return values of
//! [`group_annotations_by_path`]/[`count_threads_by_path`] below — already
//! covered, nothing left to add a type for — and the other 9
//! (`draft`, `error`, `startDraft`, `cancelDraft`, `submitDraft`,
//! `onSelectedLinesChange`, `renderAnnotation`, `renderGutterUtility`,
//! `threadsFor`) are `useState`-backed values, closures over the transport
//! functions, or `ReactNode`-returning renderers — the exact hook-body
//! surface the brief separately excludes ("Do NOT port `useReviewAnnotations`
//! itself"). Porting `ReviewAnnotationLayer` as a literal struct would mean
//! either reintroducing that presentation surface under a different name, or
//! shipping a struct with 9 of 11 fields that cannot be filled in outside a
//! hook body — neither is what "port the pure-helper half" asks for. Not
//! ported; recorded here rather than silently dropped, since the brief named
//! it explicitly.

use std::collections::HashMap;

use super::state::{ReviewThread, ThreadSide};

/// The id of the thread being composed but not yet posted. Mirrors
/// `DRAFT_THREAD_ID`.
///
/// The composer is an annotation like any other so it appears exactly where
/// the finished comment will; nothing the server ever returns can collide
/// with this id.
pub const DRAFT_THREAD_ID: &str = "__crowbar-draft-thread__";

/// Mirrors `isDraftThread`.
#[must_use]
pub fn is_draft_thread(thread: &ReviewThread) -> bool {
    thread.id == DRAFT_THREAD_ID
}

/// Which half of a diff render an annotation attaches to. The renderer's own
/// two-sided line addressing, as opposed to [`ThreadSide`]'s `old`/`new`
/// domain vocabulary — see [`to_annotation_side`]/[`to_thread_side`].
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum AnnotationSide {
    Deletions,
    Additions,
}

// ── The side map ────────────────────────────────────────────────────
//
// A thread records old/new; the renderer speaks deletions/additions. The two
// mappings are written out rather than derived because an inversion here is
// invisible in every round trip and puts every comment on the wrong half of
// the diff — mirrors the TS source's own comment verbatim.

/// Mirrors `toAnnotationSide`.
#[must_use]
pub fn to_annotation_side(side: ThreadSide) -> AnnotationSide {
    match side {
        ThreadSide::Old => AnnotationSide::Deletions,
        ThreadSide::New => AnnotationSide::Additions,
    }
}

/// Mirrors `toThreadSide`.
#[must_use]
pub fn to_thread_side(side: AnnotationSide) -> ThreadSide {
    match side {
        AnnotationSide::Deletions => ThreadSide::Old,
        AnnotationSide::Additions => ThreadSide::New,
    }
}

/// A native, `@pierre/diffs`-free replacement for the generic
/// `DiffLineAnnotation<T>` — see the module doc for what was assumed about
/// its shape.
#[derive(Debug, Clone, PartialEq)]
pub struct DiffLineAnnotation<T> {
    pub side: AnnotationSide,
    pub line_number: i64,
    pub metadata: T,
}

/// One review thread, positioned on one side of one line of one file.
/// Mirrors TS `ReviewAnnotation = DiffLineAnnotation<ReviewThread>`.
pub type ReviewAnnotation = DiffLineAnnotation<ReviewThread>;

/// Mirrors `threadToAnnotation`.
///
/// Takes `&ReviewThread` and clones into `metadata`, rather than taking
/// ownership: TS stores the *same object reference* in `metadata` (an alias,
/// not a copy), which has no direct Rust equivalent for an owned value type.
/// Cloning is also what the sibling array-consuming functions in this file
/// need — `readonly ReviewThread[]` in the TS signatures of
/// `groupAnnotationsByPath`/`countThreadsByPath` is a deliberate "does not
/// consume its input" signal this port keeps by borrowing throughout.
#[must_use]
pub fn thread_to_annotation(thread: &ReviewThread) -> ReviewAnnotation {
    DiffLineAnnotation {
        side: to_annotation_side(thread.side),
        line_number: thread.line_number,
        metadata: thread.clone(),
    }
}

/// Mirrors `annotationToThread`. See [`thread_to_annotation`]'s doc for why
/// this borrows and clones rather than taking ownership.
#[must_use]
pub fn annotation_to_thread(annotation: &ReviewAnnotation) -> ReviewThread {
    annotation.metadata.clone()
}

/// Annotations for every file that has any, ordered by line within each
/// file. Mirrors `groupAnnotationsByPath`.
///
/// Returns a [`HashMap`], not the TS source's insertion-ordered `Map` — every
/// caller looks a path up by key (`annotationsByPath.get(path)`); nothing
/// reads the map's own iteration order. Same convention as
/// `crate::git::build_git_folder_tree`'s `GitFolderNode::folders`.
#[must_use]
pub fn group_annotations_by_path(threads: &[ReviewThread]) -> HashMap<String, Vec<ReviewAnnotation>> {
    let mut by_path: HashMap<String, Vec<&ReviewThread>> = HashMap::new();
    for thread in threads {
        by_path.entry(thread.file_path.clone()).or_default().push(thread);
    }

    let mut grouped = HashMap::with_capacity(by_path.len());
    for (path, mut list) in by_path {
        list.sort_by_key(|t| t.line_number);
        grouped.insert(path, list.into_iter().map(thread_to_annotation).collect());
    }
    grouped
}

/// How many threads each file carries.
///
/// Counted from the threads list alone, so it is right for files the diff
/// has not loaded — which is the entire reason it exists (mirrors the TS
/// source's own comment). Mirrors `countThreadsByPath`.
#[must_use]
pub fn count_threads_by_path(threads: &[ReviewThread]) -> HashMap<String, usize> {
    let mut counts = HashMap::new();
    for thread in threads {
        *counts.entry(thread.file_path.clone()).or_insert(0) += 1;
    }
    counts
}

#[cfg(test)]
mod tests {
    use super::{
        AnnotationSide, DRAFT_THREAD_ID, ReviewAnnotation, annotation_to_thread,
        count_threads_by_path, group_annotations_by_path, is_draft_thread, thread_to_annotation,
        to_annotation_side, to_thread_side,
    };
    use crate::review::state::{ReviewThread, ThreadSide};

    fn thread(id: &str, file_path: &str, line_number: i64, side: ThreadSide) -> ReviewThread {
        ReviewThread {
            id: id.to_string(),
            file_path: file_path.to_string(),
            line_number,
            start_line: line_number,
            end_line: line_number,
            side,
            messages: Vec::new(),
            is_resolved: false,
        }
    }

    // --- ported from web/src/__tests__/features/git/components/diff/use-review-annotations.test.tsx ---

    #[test]
    fn maps_a_threads_old_side_to_deletions_and_its_new_side_to_additions() {
        assert_eq!(to_annotation_side(ThreadSide::Old), AnnotationSide::Deletions);
        assert_eq!(to_annotation_side(ThreadSide::New), AnnotationSide::Additions);
    }

    #[test]
    fn maps_deletions_back_to_old_and_additions_back_to_new() {
        assert_eq!(to_thread_side(AnnotationSide::Deletions), ThreadSide::Old);
        assert_eq!(to_thread_side(AnnotationSide::Additions), ThreadSide::New);
    }

    #[test]
    fn thread_to_annotation_carries_the_line_number_and_the_thread_as_metadata() {
        let t = thread("thread-1", "src/pkg/file0.ts", 42, ThreadSide::New);
        let annotation = thread_to_annotation(&t);

        assert_eq!(annotation.line_number, 42);
        assert_eq!(annotation.side, AnnotationSide::Additions);
        // TS asserts `.toBe(thread)` — reference identity. There is no Rust
        // analog of that for an owned value type; the closest honest
        // adaptation is that the clone carries the exact same field values.
        assert_eq!(annotation.metadata, t);
    }

    #[test]
    fn places_an_old_side_thread_on_the_deletions_half() {
        let t = thread("thread-1", "src/pkg/file0.ts", 2, ThreadSide::Old);
        assert_eq!(thread_to_annotation(&t).side, AnnotationSide::Deletions);
    }

    #[test]
    fn round_trips_a_thread_through_an_annotation_and_back_on_both_sides() {
        for side in [ThreadSide::Old, ThreadSide::New] {
            let t = thread("thread-x", "src/pkg/file0.ts", 7, side);
            assert_eq!(annotation_to_thread(&thread_to_annotation(&t)), t);
        }
    }

    #[test]
    fn round_trips_an_annotation_through_a_thread_and_back_on_both_sides() {
        for side in [AnnotationSide::Deletions, AnnotationSide::Additions] {
            let annotation = ReviewAnnotation {
                side,
                line_number: 11,
                metadata: thread("thread-y", "src/pkg/file0.ts", 11, to_thread_side(side)),
            };
            assert_eq!(thread_to_annotation(&annotation_to_thread(&annotation)), annotation);
        }
    }

    #[test]
    fn groups_annotations_under_each_thread_file_path_ordered_by_line() {
        let a = thread("a", "src/a.ts", 9, ThreadSide::New);
        let b = thread("b", "src/a.ts", 3, ThreadSide::New);
        let c = thread("c", "src/b.ts", 1, ThreadSide::Old);

        let grouped = group_annotations_by_path(&[a, b, c]);

        let mut paths: Vec<&String> = grouped.keys().collect();
        paths.sort();
        assert_eq!(paths, vec!["src/a.ts", "src/b.ts"]);
        let a_lines: Vec<i64> = grouped["src/a.ts"].iter().map(|x| x.line_number).collect();
        assert_eq!(a_lines, vec![3, 9]);
        assert_eq!(grouped["src/b.ts"][0].side, AnnotationSide::Deletions);
    }

    #[test]
    fn counts_threads_per_file_including_files_the_diff_never_materialises() {
        let counts = count_threads_by_path(&[
            thread("a", "src/a.ts", 1, ThreadSide::New),
            thread("b", "src/a.ts", 2, ThreadSide::New),
            thread("c", "never/materialised.ts", 1, ThreadSide::New),
        ]);

        assert_eq!(counts["src/a.ts"], 2);
        assert_eq!(counts["never/materialised.ts"], 1);
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn is_draft_thread_true_only_for_the_draft_sentinel_id() {
        let draft = thread(DRAFT_THREAD_ID, "src/a.ts", 1, ThreadSide::New);
        let real = thread("real-id", "src/a.ts", 1, ThreadSide::New);
        assert!(is_draft_thread(&draft));
        assert!(!is_draft_thread(&real));
    }

    #[test]
    fn group_annotations_by_path_is_empty_for_no_threads() {
        assert!(group_annotations_by_path(&[]).is_empty());
    }

    #[test]
    fn count_threads_by_path_is_empty_for_no_threads() {
        assert!(count_threads_by_path(&[]).is_empty());
    }
}
