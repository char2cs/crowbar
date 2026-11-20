//! What the demo data answers about its files and changes.

use crate::fixtures::demo_shell::DemoShell;
use crate::fixtures::rows::MAIN;
use crate::shell::ChangedFile;
use crate::shell::Changes;
use crate::shell::ChangesSource;
use crate::shell::Expansion;
use crate::shell::FileAccess;
use crate::shell::FileKind;
use crate::shell::FileRow;
use crate::shell::FilesSource;
use crate::shell::FolderPhase;
use crate::shell::FolderSource;
use crate::shell::GitStatus;
use crate::shell::MergeBox;

impl FilesSource for DemoShell {
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

impl ChangesSource for DemoShell {
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
            ..Changes::default()
        }
    }
}

impl FileAccess for DemoShell {}

impl FolderSource for DemoShell {
    fn folder_phase(&self) -> FolderPhase {
        FolderPhase::Listed
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_files_and_changes_describe_one_workspace() {
        let data = DemoShell::reference();
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
