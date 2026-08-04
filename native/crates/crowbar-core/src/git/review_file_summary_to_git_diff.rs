//! Project the files-only branch-review summary
//! ([`crowbar_proto::domain_git::ReviewFileSummary`]) into the sidebar-tree
//! [`GitDiff`] shape.
//!
//! Ported from
//! `web/src/features/git/utils/review-file-summary-to-git-diff.ts`. Unlike
//! [`super::git_status_to_changed_files`]'s raw-status projection, the
//! summary carries per-file `additions`/`deletions`, so the +N/-N badges
//! render, and its `uncommitted` flag is honest: committed-only files map to
//! `Some(false)` (no "uncommitted" badge) while working-tree changes stay
//! flagged `Some(true)`.
//!
//! `lines` is left empty on purpose: the summary has no line content — that
//! is the whole point, so the sidebar never pays for the line-level diff.
//! Binary files carry `-1` counts (mirrors `ReviewFileSummary`'s own doc
//! comment); nothing here hides that, matching the TS source, whose own
//! comment notes the -1 convention is hidden by the *renderer*
//! (`GitFileItem`, which only shows counts `> 0`), not by this projection.

use crowbar_proto::domain_git::{GitFileStatus, ReviewFileSummary};

use super::types::GitDiff;

/// Mirrors `reviewFilesSummaryToChangedFiles`. See
/// `git_status_to_changed_files`'s doc comment for why this crate does not
/// reproduce the TS source's "stable empty array" identity trick.
#[must_use]
pub fn review_files_summary_to_changed_files(files: &[ReviewFileSummary]) -> Vec<GitDiff> {
    files.iter().map(review_file_summary_to_git_diff).collect()
}

fn review_file_summary_to_git_diff(file: &ReviewFileSummary) -> GitDiff {
    GitDiff {
        file_path: file.path.clone(),
        old_path: file.old_path.clone(),
        // 'untracked' has no committed counterpart — render it as an
        // addition, matching how the full branch diff surfaces brand-new
        // files (is_new).
        is_new: matches!(file.status, GitFileStatus::Added | GitFileStatus::Untracked),
        is_deleted: matches!(file.status, GitFileStatus::Deleted),
        is_renamed: matches!(file.status, GitFileStatus::Renamed),
        additions: Some(file.additions),
        deletions: Some(file.deletions),
        uncommitted: Some(file.uncommitted),
        lines: Vec::new(),
    }
}

#[cfg(test)]
mod tests {
    use super::review_files_summary_to_changed_files;
    use crowbar_proto::domain_git::{GitFileStatus, ReviewFileSummary};

    fn summary(path: &str, status: GitFileStatus) -> ReviewFileSummary {
        ReviewFileSummary {
            path: path.to_string(),
            old_path: None,
            status,
            additions: 0,
            deletions: 0,
            uncommitted: false,
            staged: false,
        }
    }

    // --- ported from web/src/__tests__/features/git/utils/review-file-summary-to-git-diff.test.ts ---

    #[test]
    fn returns_empty_for_no_files() {
        assert_eq!(review_files_summary_to_changed_files(&[]), vec![]);
    }

    #[test]
    fn maps_added_and_untracked_to_is_new() {
        let out = review_files_summary_to_changed_files(&[
            ReviewFileSummary {
                additions: 5,
                ..summary("a.ts", GitFileStatus::Added)
            },
            ReviewFileSummary {
                uncommitted: true,
                ..summary("b.ts", GitFileStatus::Untracked)
            },
        ]);
        assert!(out[0].is_new);
        assert_eq!(out[0].additions, Some(5));
        assert!(out[1].is_new);
        assert_eq!(out[1].uncommitted, Some(true));
    }

    #[test]
    fn maps_deleted_and_modified_with_their_counts_and_uncommitted_flag() {
        let out = review_files_summary_to_changed_files(&[
            ReviewFileSummary {
                deletions: 9,
                ..summary("gone.ts", GitFileStatus::Deleted)
            },
            ReviewFileSummary {
                additions: 3,
                deletions: 1,
                uncommitted: true,
                ..summary("keep.ts", GitFileStatus::Modified)
            },
        ]);
        assert!(out[0].is_deleted);
        assert_eq!(out[0].deletions, Some(9));
        assert!(!out[1].is_new);
        assert!(!out[1].is_deleted);
        assert!(!out[1].is_renamed);
        assert_eq!(out[1].additions, Some(3));
        assert_eq!(out[1].uncommitted, Some(true));
    }

    #[test]
    fn maps_renamed_carrying_the_old_path() {
        let out = review_files_summary_to_changed_files(&[ReviewFileSummary {
            old_path: Some("old.ts".to_string()),
            ..summary("new.ts", GitFileStatus::Renamed)
        }]);
        assert!(out[0].is_renamed);
        assert_eq!(out[0].old_path.as_deref(), Some("old.ts"));
        assert_eq!(out[0].file_path, "new.ts");
    }

    #[test]
    fn preserves_the_negative_one_binary_counts() {
        let out = review_files_summary_to_changed_files(&[ReviewFileSummary {
            additions: -1,
            deletions: -1,
            ..summary("logo.png", GitFileStatus::Modified)
        }]);
        assert_eq!(out[0].additions, Some(-1));
        assert_eq!(out[0].deletions, Some(-1));
    }

    #[test]
    fn committed_only_files_carry_uncommitted_false_no_badge() {
        let out = review_files_summary_to_changed_files(&[ReviewFileSummary {
            additions: 4,
            uncommitted: false,
            ..summary("src/x.ts", GitFileStatus::Modified)
        }]);
        assert_eq!(out[0].uncommitted, Some(false));
        assert_eq!(out[0].additions, Some(4));
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn conflicted_and_other_statuses_are_not_new_deleted_or_renamed() {
        let out = review_files_summary_to_changed_files(&[
            summary("a.ts", GitFileStatus::Conflicted),
            summary("b.ts", GitFileStatus::Other(String::new())),
        ]);
        for entry in &out {
            assert!(!entry.is_new);
            assert!(!entry.is_deleted);
            assert!(!entry.is_renamed);
        }
    }
}
