//! The daemon's `GET chats/:id/git/status` answer.

use serde::Deserialize;
use sidebar::{ChangedFile, Changes};

use crate::git_changes::status_of;
use crate::git_file_body::GitFileBody;
use crate::line_counts::LineCounts;

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
pub(crate) struct GitStatusBody {
    branch: String,
    files: Vec<GitFileBody>,
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
        }
    }
}

fn changed_file(
    file: &GitFileBody,
    counts: &LineCounts,
) -> ChangedFile {
    let (added, deleted) = counts.get(&file.path).copied().unwrap_or_default();
    ChangedFile {
        path: file.path.clone(),
        status: status_of(&file.status),
        added,
        deleted,
    }
}

#[cfg(test)]
mod tests {
    use sidebar::GitStatus;

    use super::*;

    #[test]
    fn the_daemon_status_becomes_the_panel_model() {
        let body: GitStatusBody = serde_json::from_str(
            r#"{"branch":"main","ahead":1,"files":[{"path":"a","status":"untracked"},{"path":"b","status":"renamed"}]}"#,
        )
        .unwrap();
        let counts = LineCounts::from([("b".to_owned(), (4, 2))]);
        let changes = body.changes(&counts);
        assert_eq!(changes.branch, "main");
        assert_eq!(changes.files[0].status, GitStatus::Untracked);
        assert_eq!(changes.files[1].status, GitStatus::Modified);
        assert_eq!((changes.files[1].added, changes.files[1].deleted), (4, 2));
    }
}
