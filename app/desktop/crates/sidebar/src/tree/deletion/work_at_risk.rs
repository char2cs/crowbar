//! Work that exists nowhere else, named in the delete confirmation.

/// What deleting one branch would lose.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct WorkAtRisk {
    /// The branch name.
    pub branch: String,
    /// Files with uncommitted changes.
    pub uncommitted_files: u32,
    /// Commits on no other branch, remote or tag.
    pub unmerged_commits: u32,
}

fn plural(count: u32) -> &'static str {
    if count == 1 {
        return "";
    }
    "s"
}

impl WorkAtRisk {
    /// "2 uncommitted files · 1 unmerged commit".
    pub fn summary(&self) -> String {
        let parts = [
            (self.uncommitted_files, "uncommitted file"),
            (self.unmerged_commits, "unmerged commit"),
        ];
        let shown: Vec<String> = parts
            .iter()
            .filter(|(count, _)| *count > 0)
            .map(|(count, noun)| format!("{count} {noun}{}", plural(*count)))
            .collect();
        shown.join(" · ")
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn risk(
        files: u32,
        commits: u32,
    ) -> WorkAtRisk {
        WorkAtRisk {
            branch: "b".into(),
            uncommitted_files: files,
            unmerged_commits: commits,
        }
    }

    #[test]
    fn summary_pluralises_and_skips_zero_counts() {
        let cases = [
            (2, 1, "2 uncommitted files · 1 unmerged commit"),
            (1, 0, "1 uncommitted file"),
            (0, 3, "3 unmerged commits"),
            (0, 0, ""),
        ];
        for (files, commits, expected) in cases {
            let summary = risk(files, commits).summary();
            assert_eq!(summary, expected, "{files} files {commits} commits");
        }
    }
}
