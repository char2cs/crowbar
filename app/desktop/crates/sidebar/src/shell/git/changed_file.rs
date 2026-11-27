use crate::shell::listing::GitStatus;

#[derive(Debug, Clone, PartialEq, Eq, Default)]
pub struct ChangedFile {
    pub path: String,
    pub status: GitStatus,
    pub added: u32,
    pub deleted: u32,
    pub signature: String,
}

impl ChangedFile {
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
