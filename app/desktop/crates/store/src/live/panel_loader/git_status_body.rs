//! The daemon's `GET chats/:id/git/status` answer.

mod file_body;

use serde::Deserialize;
use sidebar::{ChangedFile, Changes};

use crate::live::panel_loader::git_status_body::file_body::FileBody;
use crate::live::panel_loader::line_counts::LineCounts;

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

fn changed_file(
    file: &FileBody,
    counts: &LineCounts,
) -> ChangedFile {
    let (added, deleted) = counts.get(&file.path).copied().unwrap_or_default();
    ChangedFile {
        path: file.path.clone(),
        status: file.git_status(),
        added,
        deleted,
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
}
