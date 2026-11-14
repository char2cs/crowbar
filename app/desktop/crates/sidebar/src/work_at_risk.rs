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
            .map(|(count, noun)| format!("{count} {noun}{}", if *count == 1 { "" } else { "s" }))
            .collect();
        shown.join(" · ")
    }
}

#[cfg(test)]
#[path = "work_at_risk_tests.rs"]
mod tests;
