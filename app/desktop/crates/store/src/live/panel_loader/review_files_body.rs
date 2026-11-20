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
        "renamed" => GitStatus::Renamed,
        _ => GitStatus::Modified,
    }
}

fn line_count(raw: i64) -> u32 {
    u32::try_from(raw).unwrap_or_default()
}

fn changed_file(file: SummaryFile) -> ChangedFile {
    let status = status_of(&file.status);
    let old_path = file.old_path.as_deref().unwrap_or_default();
    let counts = (file.additions, file.deletions);
    ChangedFile {
        signature: ChangedFile::signature_of(status, old_path, counts),
        status,
        path: file.path,
        added: line_count(file.additions),
        deleted: line_count(file.deletions),
    }
}

impl ReviewFilesBody {
    pub(crate) fn files(self) -> Vec<ChangedFile> {
        self.files.into_iter().map(changed_file).collect()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    const WORDS: [(&str, GitStatus); 4] = [
        ("untracked", GitStatus::Added),
        ("added", GitStatus::Added),
        ("deleted", GitStatus::Deleted),
        ("renamed", GitStatus::Renamed),
    ];

    #[test]
    fn a_binary_file_still_lists_and_each_file_carries_the_reviews_signature() {
        let body: Result<ReviewFilesBody, _> = serde_json::from_str(
            r#"{"files":[{"path":"logo.png","status":"modified","additions":-1,"deletions":-1},{"path":"b.ts","old_path":"a.ts","status":"renamed","additions":2,"deletions":1}]}"#,
        );
        let files = body.map_or_else(|_| Vec::new(), ReviewFilesBody::files);
        let listed: Vec<String> = files
            .iter()
            .map(|file| {
                format!(
                    "{} +{} -{} {}",
                    file.path, file.added, file.deleted, file.signature
                )
            })
            .collect();
        assert_eq!(listed, ["logo.png +0 -0 M||-1|-1", "b.ts +2 -1 R|a.ts|2|1"]);
    }

    #[test]
    fn a_new_file_reads_as_added_as_the_branch_diff_shows_it() {
        for (word, status) in WORDS {
            assert_eq!(status_of(word), status, "{word}");
        }
    }
}
