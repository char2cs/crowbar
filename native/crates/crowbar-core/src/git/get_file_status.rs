//! Ported from `web/src/features/git/utils/git-diff-helpers.ts`'s
//! `getFileStatus`.
//!
//! # Where this lives, and why not `crowbar-diff`
//!
//! `getFileStatus` sits in the same TS file as `getImgSrc` (presentation, a
//! `data:` URI formatter — not ported, see
//! `native/mapping/tier-a-denominator.md`'s Deliverable 3 §2 skip list), and
//! both are reached only through `git-diff-image.tsx`, itself reachable only
//! inside the branch-review surface (`review-code-view.tsx`). That surface's
//! *other* pure region — `partitionReviewFiles`/`buildPlaceholderFileDiff`
//! and their private helpers — is assigned to `crowbar-diff` by the mapping
//! doc's own §1/§2 crate-boundary finding (placeholder-hunk geometry is
//! virtualiser sizing, not domain structure). `getFileStatus` is not part of
//! that finding: §2's own text is explicit that "file-status classification
//! (`is_new`/`is_deleted`/`is_renamed` → label) [is] genuine, tiny,
//! **git-model** logic, counted in §1" — and §1's own "genuine, portable
//! git-model logic" list names `getFileStatus` directly, alongside the two
//! near-duplicate classifiers already ported here
//! ([`super::git_status_to_changed_files`], [`super::review_file_summary_to_git_diff`]):
//! "a third, smaller restatement of the same `is_new`/`is_deleted`/
//! `is_renamed` → label mapping already done twice above. Three near-duplicate
//! implementations... is itself a finding: a single `crowbar-core` type
//! should collapse all three." So this function lands beside its two
//! siblings, in `crowbar-core::git`, not in `crowbar-diff` — a correction to
//! this item's own brief, which grouped `getFileStatus` with the
//! `crowbar-diff`-bound placeholder algebra under one crate-boundary
//! citation that, read closely, does not actually cover it. No consolidation
//! of the three near-duplicate classifiers is attempted here; that is a
//! separate finding this item does not have scope to act on.
//!
//! Operates on the same [`super::types::GitDiff`] the other two classifiers
//! already use — `getFileStatus`'s real caller
//! (`git-diff-image.tsx`) receives exactly this shape.

use super::types::GitDiff;

/// Mirrors `getFileStatus`'s `is_new` > `is_deleted` > `is_renamed` >
/// "modified" precedence. Returns a plain `&'static str`, matching the TS
/// source's own loose `: string` return type (not a literal union) — three
/// of [`crowbar_proto::domain_git::GitFileStatus`]'s six variants
/// (`conflicted`, `untracked`, the open `Other` case) are not reachable from
/// this boolean-flag shape at all, so this is not that enum restated.
#[must_use]
pub fn get_file_status(diff: &GitDiff) -> &'static str {
    if diff.is_new {
        return "added";
    }
    if diff.is_deleted {
        return "deleted";
    }
    if diff.is_renamed {
        return "renamed";
    }
    "modified"
}

#[cfg(test)]
mod tests {
    use super::{GitDiff, get_file_status};

    fn diff(is_new: bool, is_deleted: bool, is_renamed: bool) -> GitDiff {
        GitDiff {
            is_new,
            is_deleted,
            is_renamed,
            ..Default::default()
        }
    }

    // --- new: git-diff-helpers.ts's getFileStatus has zero existing TS
    // tests (tier-a-denominator.md §1's own "Tests" table notes this
    // directly: "Zero test files exist for git-diff-helpers.ts's
    // getFileStatus — it is exercised only incidentally through the two
    // projection tests above"), so none of the cases below are ported from
    // a TS suite; all are authored fresh against the TS source's four
    // branches. ---

    #[test]
    fn new_file_is_added() {
        assert_eq!(get_file_status(&diff(true, false, false)), "added");
    }

    #[test]
    fn deleted_file_is_deleted() {
        assert_eq!(get_file_status(&diff(false, true, false)), "deleted");
    }

    #[test]
    fn renamed_file_is_renamed() {
        assert_eq!(get_file_status(&diff(false, false, true)), "renamed");
    }

    #[test]
    fn no_flags_set_is_modified() {
        assert_eq!(get_file_status(&diff(false, false, false)), "modified");
    }

    #[test]
    fn is_new_wins_over_every_other_flag() {
        // Mirrors the TS source's checked-in-order `if` chain: is_new is
        // checked first, so a (contradictory, in practice never-produced-by-
        // the-daemon) file carrying every flag still reads as "added".
        assert_eq!(get_file_status(&diff(true, true, true)), "added");
    }

    #[test]
    fn is_deleted_wins_over_is_renamed_when_is_new_is_false() {
        assert_eq!(get_file_status(&diff(false, true, true)), "deleted");
    }
}
