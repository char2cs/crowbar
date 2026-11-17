//! What the demo data answers about its files and changes.

use crate::changed_file::ChangedFile;
use crate::changes::Changes;
use crate::changes_source::ChangesSource;
use crate::demo_data::DemoData;
use crate::demo_rows::MAIN;
use crate::expansion::Expansion;
use crate::file_access::FileAccess;
use crate::file_kind::FileKind;
use crate::file_row::FileRow;
use crate::files_source::FilesSource;
use crate::git_status::GitStatus;
use crate::merge_box::MergeBox;

impl FilesSource for DemoData {
    fn files(&self) -> Vec<FileRow> {
        let file = |path: &str, name: &str, depth, kind, status| FileRow {
            path: path.to_owned(),
            name: name.to_owned(),
            depth,
            kind,
            status,
        };
        vec![
            file(
                "src",
                "src",
                0,
                FileKind::Directory(Expansion::Expanded),
                GitStatus::Clean,
            ),
            file(
                "src/main.rs",
                "main.rs",
                1,
                FileKind::File,
                GitStatus::Modified,
            ),
            file("src/lib.rs", "lib.rs", 1, FileKind::File, GitStatus::Added),
            file(
                "README.md",
                "README.md",
                0,
                FileKind::File,
                GitStatus::Clean,
            ),
        ]
    }

    fn workspace_root(&self) -> Option<String> {
        Some("/work/demo".to_owned())
    }
}

impl ChangesSource for DemoData {
    fn merge_box(&self) -> Option<MergeBox> {
        self.merge.clone()
    }

    fn changes(&self) -> Changes {
        let file = ChangedFile {
            path: "src/lib.rs".into(),
            status: GitStatus::Added,
            added: 1,
            deleted: 0,
        };
        Changes {
            branch: "inv-rb".into(),
            parent: Some(MAIN.into()),
            files: vec![file],
        }
    }
}

impl FileAccess for DemoData {}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_files_and_changes_describe_one_workspace() {
        let data = DemoData::reference();
        let files = data.files();
        assert!(files.iter().any(|row| row.path == "src/lib.rs"));
        let changes = data.changes();
        assert!(
            changes
                .files
                .iter()
                .all(|changed| files.iter().any(|row| row.path == changed.path))
        );
        assert_eq!(data.workspace_root().as_deref(), Some("/work/demo"));
        assert_eq!(data.merge_box(), None);
    }
}
