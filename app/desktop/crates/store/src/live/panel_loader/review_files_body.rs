//! The daemon's `GET chats/:id/review/files` answer: every file the branch changed against its
//! parent, committed or not, with its line counts.

mod summary_file;

use serde::Deserialize;
use sidebar::{ChangedFile, GitStatus};

use crate::live::panel_loader::review_files_body::summary_file::SummaryFile;

#[derive(Debug, Default, Deserialize)]
#[serde(default)]
pub(crate) struct ReviewFilesBody {
    files: Vec<SummaryFile>,
}

fn status_of(word: &str) -> GitStatus {
    match word {
        "added" | "untracked" => GitStatus::Added,
        "deleted" => GitStatus::Deleted,
        _ => GitStatus::Modified,
    }
}

fn line_count(raw: i64) -> u32 {
    u32::try_from(raw).unwrap_or_default()
}

impl ReviewFilesBody {
    pub(crate) fn files(self) -> Vec<ChangedFile> {
        self.files
            .into_iter()
            .map(|file| ChangedFile {
                status: status_of(&file.status),
                path: file.path,
                added: line_count(file.additions),
                deleted: line_count(file.deletions),
            })
            .collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const WORDS: [(&str, GitStatus); 4] = [
        ("untracked", GitStatus::Added),
        ("added", GitStatus::Added),
        ("deleted", GitStatus::Deleted),
        ("renamed", GitStatus::Modified),
    ];

    #[test]
    fn a_binary_file_without_line_counts_still_lists() {
        let body: Result<ReviewFilesBody, _> = serde_json::from_str(
            r#"{"files":[{"path":"logo.png","status":"modified","additions":-1,"deletions":-1}]}"#,
        );
        let files = body.map_or_else(|_| Vec::new(), ReviewFilesBody::files);
        let listed: Vec<(&str, u32, u32)> = files
            .iter()
            .map(|file| (file.path.as_str(), file.added, file.deleted))
            .collect();
        assert_eq!(listed, [("logo.png", 0, 0)]);
    }

    #[test]
    fn a_new_file_reads_as_added_as_the_branch_diff_shows_it() {
        for (word, status) in WORDS {
            assert_eq!(status_of(word), status, "{word}");
        }
    }
}
