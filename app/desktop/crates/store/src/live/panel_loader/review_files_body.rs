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

impl ReviewFilesBody {
    pub(crate) fn files(self) -> Vec<ChangedFile> {
        self.files
            .into_iter()
            .map(|file| ChangedFile {
                status: status_of(&file.status),
                path: file.path,
                added: file.additions,
                deleted: file.deletions,
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
    fn a_new_file_reads_as_added_as_the_branch_diff_shows_it() {
        for (word, status) in WORDS {
            assert_eq!(status_of(word), status, "{word}");
        }
    }
}
