//! The daemon's `GET chats/:id/git/status` answer.

mod file_body;
pub(super) mod git_diff_body;

use std::collections::HashMap;

use serde::Deserialize;
use sidebar::{ChangedFile, Changes};

use crate::live::panel_loader::git_status_body::file_body::FileBody;
use crate::live::panel_loader::git_status_body::git_diff_body::GitDiffBody;

pub(crate) type LineCounts = HashMap<String, (u32, u32)>;

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
pub(crate) struct GitStatusBody {
    branch: String,
    files: Vec<FileBody>,
}

impl GitStatusBody {
    pub(crate) fn staged_paths(&self) -> std::collections::HashSet<String> {
        self.files
            .iter()
            .filter(|file| file.staged)
            .map(|file| file.path.clone())
            .collect()
    }

    pub(crate) fn changes(
        &self,
        counts: &LineCounts,
    ) -> Changes {
        Changes {
            branch: self.branch.clone(),
            parent: None,
            files: self
                .files
                .iter()
                .map(|file| changed_file(file, counts))
                .collect(),
            ..Changes::default()
        }
    }
}

pub(crate) fn line_counts(diffs: &[GitDiffBody]) -> LineCounts {
    let mut counts = LineCounts::new();
    for diff in diffs {
        let held = counts.entry(diff.file_path.clone()).or_default();
        *held = (
            held.0.saturating_add(diff.additions),
            held.1.saturating_add(diff.deletions),
        );
    }
    counts
}

fn changed_file(
    file: &FileBody,
    counts: &LineCounts,
) -> ChangedFile {
    let (added, deleted) = counts.get(&file.path).copied().unwrap_or_default();
    let status = file.git_status();
    let signature = ChangedFile::signature_of(status, "", (added.into(), deleted.into()));
    ChangedFile {
        path: file.path.clone(),
        status,
        added,
        deleted,
        signature,
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_daemon_status_becomes_the_panel_model() {
        let body: GitStatusBody = serde_json::from_str(
            r#"{"branch":"main","ahead":1,"files":[{"path":"a","status":"untracked"},{"path":"b","status":"renamed"}]}"#,
        )
        .unwrap();
        let counts = LineCounts::from([("b".to_owned(), (4, 2))]);
        let changes = body.changes(&counts);
        let files: Vec<String> = changes
            .files
            .iter()
            .map(|file| {
                format!(
                    "{} {:?} +{} -{}",
                    file.path, file.status, file.added, file.deleted
                )
            })
            .collect();
        assert_eq!(changes.branch, "main");
        assert_eq!(files, ["a Untracked +0 -0", "b Renamed +4 -2"]);
    }

    #[test]
    fn a_file_in_both_diffs_sums_its_lines() {
        let diffs: Vec<GitDiffBody> = serde_json::from_str(
            r#"[{"file_path":"a","additions":2,"deletions":1},{"file_path":"a","additions":3},{"file_path":"b"}]"#,
        )
        .unwrap();
        let counts = line_counts(&diffs);
        assert_eq!(counts["a"], (5, 1));
        assert_eq!(counts["b"], (0, 0));
    }
}
