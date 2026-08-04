//! Project the cheap working-tree git status into the sidebar-tree
//! [`GitDiff`] shape, WITHOUT the full line-level branch diff.
//!
//! Ported from `web/src/features/git/utils/git-status-to-changed-files.ts`.
//! This drives the always-mounted sidebar changed-files list while the
//! Branch Review pane is closed, so the more expensive review-summary fetch
//! ([`super::review_file_summary_to_git_diff`]) only runs while review is
//! actually open.
//!
//! The working-tree status payload
//! ([`crowbar_proto::api_v0_dto::GitFileDTO`]: path/status/staged) carries no
//! per-file line counts, so `additions`/`deletions` are `None` on every
//! entry — the +N/-N badges only appear once the full diff/summary is
//! available. `uncommitted` is `None` too, for a different reason: every
//! entry here already IS an uncommitted working-tree change (that's the
//! whole source), so there's nothing to distinguish — see
//! `crate::git::types::GitDiff`'s doc comment for why `None` here does not
//! mean the same thing as `Some(false)`.
//!
//! `git status` can list the same path twice (a staged entry AND an
//! unstaged entry); the branch diff shows one row per file, so this dedupes
//! by path — first occurrence wins — to keep the tree visually identical.

use std::collections::HashSet;

use crowbar_proto::api_v0_dto::GitFileDTO;

use super::types::GitDiff;

/// Mirrors `gitStatusToChangedFiles`.
///
/// # A dropped optimisation, and why
///
/// The TS source reuses one module-level empty array (`EMPTY`) so an
/// empty-input result is referentially stable across calls — a React
/// memoization trick (a consumer keyed on identity doesn't re-render when
/// nothing changed). Nothing in `crowbar-core` renders anything or performs
/// identity-keyed memoization on owned `Vec`s; the eventual GPUI consumer's
/// re-render gating is `crowbar-state`'s `Entity<T>`/`notify()` machinery
/// (Phase 4), not this function's return value's address. Reproducing "same
/// reference" here (e.g. via a `OnceLock`/`Arc`) would add a stateful global
/// to a function the TS source deliberately keeps pure, to satisfy a
/// framework-specific concern this crate does not have.
#[must_use]
pub fn git_status_to_changed_files(files: &[GitFileDTO]) -> Vec<GitDiff> {
    let mut seen = HashSet::with_capacity(files.len());
    let mut out = Vec::new();
    for file in files {
        if !seen.insert(file.path.as_str()) {
            continue;
        }
        out.push(GitDiff {
            file_path: file.path.clone(),
            // 'untracked' has no committed counterpart — render it as an
            // addition, matching how the full branch diff surfaces
            // brand-new files (is_new).
            is_new: file.status == "added" || file.status == "untracked",
            is_deleted: file.status == "deleted",
            is_renamed: file.status == "renamed",
            ..GitDiff::default()
        });
    }
    out
}

#[cfg(test)]
mod tests {
    use super::git_status_to_changed_files;
    use crowbar_proto::api_v0_dto::GitFileDTO;

    fn file(path: &str, status: &str, staged: bool) -> GitFileDTO {
        GitFileDTO {
            path: path.to_string(),
            status: status.to_string(),
            staged,
        }
    }

    // --- ported from web/src/__tests__/features/git/utils/git-status-to-changed-files.test.ts ---

    #[test]
    fn returns_empty_for_no_files() {
        assert_eq!(git_status_to_changed_files(&[]), vec![]);
    }

    #[test]
    fn maps_every_git_status_kind_onto_the_git_diff_is_flags() {
        let out = git_status_to_changed_files(&[
            file("src/mod.ts", "modified", false),
            file("src/new.ts", "added", false),
            file("src/gone.ts", "deleted", false),
            file("src/moved.ts", "renamed", false),
            file("src/untracked.ts", "untracked", false),
        ]);

        let flags: Vec<(bool, bool, bool)> = out
            .iter()
            .map(|d| (d.is_new, d.is_deleted, d.is_renamed))
            .collect();
        assert_eq!(
            flags,
            vec![
                (false, false, false), // modified
                (true, false, false),  // added
                (false, true, false),  // deleted
                (false, false, true),  // renamed
                (true, false, false),  // untracked -> surfaces as new
            ]
        );
        assert!(out.iter().all(|d| d.lines.is_empty()));
    }

    #[test]
    fn carries_no_per_file_line_counts_or_uncommitted_flag_unavailable_from_status() {
        let out = git_status_to_changed_files(&[file("a.ts", "modified", false)]);
        assert_eq!(out[0].additions, None);
        assert_eq!(out[0].deletions, None);
        assert_eq!(out[0].uncommitted, None);
    }

    #[test]
    fn dedupes_a_path_listed_twice_staged_and_unstaged_into_a_single_entry() {
        let out = git_status_to_changed_files(&[
            file("src/a.ts", "modified", true),
            file("src/a.ts", "modified", false),
            file("src/b.ts", "modified", false),
        ]);
        let paths: Vec<&str> = out.iter().map(|d| d.file_path.as_str()).collect();
        assert_eq!(paths, vec!["src/a.ts", "src/b.ts"]);
    }

    // --- new: not exercised by the TS suite ---

    #[test]
    fn first_occurrence_wins_the_dedupe_even_when_the_first_entry_is_the_unstaged_one() {
        // The TS source's own comment says "first occurrence wins" — prove it
        // actually keeps the FIRST entry's shape (not just the first path),
        // by giving the first and second entries different statuses.
        let out = git_status_to_changed_files(&[
            file("src/a.ts", "added", false),
            file("src/a.ts", "deleted", true),
        ]);
        assert_eq!(out.len(), 1);
        assert!(out[0].is_new);
        assert!(!out[0].is_deleted);
    }
}
