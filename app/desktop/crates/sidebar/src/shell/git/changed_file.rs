//! One entry of the Changes panel.

use crate::shell::listing::GitStatus;

/// A file the branch changed.
#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct ChangedFile {
    /// Path relative to the repository.
    pub path: String,
    /// Git status.
    pub status: GitStatus,
    /// Added lines.
    pub added: u32,
    /// Deleted lines.
    pub deleted: u32,
    /// What a viewed mark holds against: see [`ChangedFile::signature_of`].
    pub signature: String,
}

impl ChangedFile {
    /// The review's `kind|old path|additions|deletions` for a file, as develop's
    /// `fileSignature` writes it: a viewed mark holds while this stays the same. Counts are the
    /// daemon's, `-1` for a binary file.
    #[must_use]
    pub fn signature_of(
        status: GitStatus,
        old_path: &str,
        (additions, deletions): (i64, i64),
    ) -> String {
        let kind = match status {
            GitStatus::Added | GitStatus::Untracked => 'A',
            GitStatus::Deleted => 'D',
            GitStatus::Renamed => 'R',
            _ => 'M',
        };
        format!("{kind}|{old_path}|{additions}|{deletions}")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    type Case = (GitStatus, &'static str, (i64, i64), &'static str);

    const CASES: [Case; 6] = [
        (GitStatus::Added, "", (18, 0), "A||18|0"),
        (GitStatus::Untracked, "", (3, 0), "A||3|0"),
        (GitStatus::Deleted, "", (0, 9), "D||0|9"),
        (GitStatus::Renamed, "src/old.ts", (1, 1), "R|src/old.ts|1|1"),
        (GitStatus::StagedModified, "", (16, 2), "M||16|2"),
        (GitStatus::Modified, "", (-1, -1), "M||-1|-1"),
    ];

    #[test]
    fn the_signature_is_the_one_the_review_writes() {
        for (status, old_path, counts, expected) in CASES {
            assert_eq!(
                ChangedFile::signature_of(status, old_path, counts),
                expected
            );
        }
    }
}
