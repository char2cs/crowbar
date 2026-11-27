pub(super) mod file_key;

use crate::explorer::file_steps::file_key::FileKey;
use crate::shell::ClipMode;
use crate::shell::Command;
use crate::shell::Expansion;
use crate::shell::FileKind;
use crate::shell::FileRow;
use crate::shell::Intent;
use crate::shell::PathText;

fn select(row: &FileRow) -> Intent {
    Intent::SelectFile(row.path.clone())
}

fn toggle(row: &FileRow) -> Intent {
    Intent::Send(Command::ToggleDirectory(row.path.clone()))
}

fn at(
    rows: &[FileRow],
    index: usize,
) -> Option<Intent> {
    rows.get(index).map(select)
}

fn right(
    rows: &[FileRow],
    index: usize,
) -> Option<Intent> {
    let row = rows.get(index)?;
    match row.kind {
        FileKind::Directory(Expansion::Collapsed) => Some(toggle(row)),
        FileKind::Directory(Expansion::Expanded) => at(rows, index + 1),
        FileKind::File => None,
    }
}

fn left(
    rows: &[FileRow],
    index: usize,
) -> Option<Intent> {
    let row = rows.get(index)?;
    if row.kind == FileKind::Directory(Expansion::Expanded) {
        return Some(toggle(row));
    }
    let parent_index = rows
        .get(..index)?
        .iter()
        .rposition(|other| other.depth < row.depth)?;
    at(rows, parent_index)
}

fn enter(row: &FileRow) -> Intent {
    match row.kind {
        FileKind::Directory(_) => toggle(row),
        FileKind::File => Intent::Send(Command::OpenFile(row.path.clone())),
    }
}

fn paste_target(row: &FileRow) -> Option<Intent> {
    let dir = match row.kind {
        FileKind::Directory(_) => row.path.as_str(),
        FileKind::File => PathText::new(&row.path).parent(),
    };
    if dir.is_empty() {
        return None;
    }
    Some(Intent::PasteFiles {
        dir: dir.to_owned(),
    })
}

fn act_on(
    row: &FileRow,
    key: FileKey,
) -> Option<Intent> {
    let path = row.path.clone();
    match key {
        FileKey::Enter => Some(enter(row)),
        FileKey::Rename => Some(Intent::StartFileRename(path)),
        FileKey::Delete => Some(Intent::RequestFileDelete {
            path,
            is_dir: matches!(row.kind, FileKind::Directory(_)),
        }),
        FileKey::Copy => Some(Intent::StageFile {
            path,
            mode: ClipMode::Copy,
        }),
        FileKey::Cut => Some(Intent::StageFile {
            path,
            mode: ClipMode::Cut,
        }),
        FileKey::Paste => paste_target(row),
        _ => None,
    }
}

impl FileKey {
    pub fn step(
        self,
        rows: &[FileRow],
        selected: Option<&str>,
    ) -> Option<Intent> {
        let last = rows.len().checked_sub(1)?;
        let index = rows
            .iter()
            .position(|row| Some(row.path.as_str()) == selected)
            .unwrap_or(0);
        match self {
            FileKey::Up => at(rows, index.saturating_sub(1)),
            FileKey::Down => at(rows, (index + 1).min(last)),
            FileKey::Home => at(rows, 0),
            FileKey::End => at(rows, last),
            FileKey::Left => left(rows, index),
            FileKey::Right => right(rows, index),
            _ => act_on(rows.get(index)?, self),
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::shell::GitStatus;

    fn row(
        path: &str,
        depth: u8,
        kind: FileKind,
    ) -> FileRow {
        FileRow {
            path: path.into(),
            name: path.into(),
            depth,
            kind,
            status: GitStatus::Clean,
        }
    }

    fn rows() -> Vec<FileRow> {
        vec![
            row("src", 0, FileKind::Directory(Expansion::Expanded)),
            row("src/a.rs", 1, FileKind::File),
            row("docs", 0, FileKind::Directory(Expansion::Collapsed)),
        ]
    }

    fn select_of(path: &str) -> Intent {
        Intent::SelectFile(path.into())
    }

    fn send(command: Command) -> Intent {
        Intent::Send(command)
    }

    #[test]
    fn keys_move_the_selection_within_the_rows() {
        type Case = (&'static str, Option<&'static str>, FileKey, Intent);
        let cases: [Case; 5] = [
            (
                "down from nothing starts at the second row",
                None,
                FileKey::Down,
                select_of("src/a.rs"),
            ),
            (
                "down stops at the end",
                Some("docs"),
                FileKey::Down,
                select_of("docs"),
            ),
            (
                "up stops at the top",
                Some("src"),
                FileKey::Up,
                select_of("src"),
            ),
            ("home", Some("docs"), FileKey::Home, select_of("src")),
            ("end", Some("src"), FileKey::End, select_of("docs")),
        ];
        for (name, selected, key, want) in cases {
            assert_eq!(key.step(&rows(), selected), Some(want), "{name}");
        }
    }

    #[test]
    fn keys_fold_and_unfold_directories() {
        type Case = (&'static str, Option<&'static str>, FileKey, Intent);
        let cases: [Case; 4] = [
            (
                "right on a folded directory unfolds it",
                Some("docs"),
                FileKey::Right,
                send(Command::ToggleDirectory("docs".into())),
            ),
            (
                "right on an open directory enters it",
                Some("src"),
                FileKey::Right,
                select_of("src/a.rs"),
            ),
            (
                "left on an open directory folds it",
                Some("src"),
                FileKey::Left,
                send(Command::ToggleDirectory("src".into())),
            ),
            (
                "left on a file selects its directory",
                Some("src/a.rs"),
                FileKey::Left,
                select_of("src"),
            ),
        ];
        for (name, selected, key, want) in cases {
            assert_eq!(key.step(&rows(), selected), Some(want), "{name}");
        }
    }

    #[test]
    fn enter_opens_and_f2_renames() {
        type Case = (&'static str, Option<&'static str>, FileKey, Intent);
        let cases: [Case; 3] = [
            (
                "enter opens a file",
                Some("src/a.rs"),
                FileKey::Enter,
                send(Command::OpenFile("src/a.rs".into())),
            ),
            (
                "enter toggles a directory",
                Some("docs"),
                FileKey::Enter,
                send(Command::ToggleDirectory("docs".into())),
            ),
            (
                "f2 renames",
                Some("src/a.rs"),
                FileKey::Rename,
                Intent::StartFileRename("src/a.rs".into()),
            ),
        ];
        for (name, selected, key, want) in cases {
            assert_eq!(key.step(&rows(), selected), Some(want), "{name}");
        }
    }

    #[test]
    fn staging_and_deleting_act_on_the_row_and_pasting_needs_a_directory() {
        let copy = Intent::StageFile {
            path: "src/a.rs".into(),
            mode: ClipMode::Copy,
        };
        let delete = Intent::RequestFileDelete {
            path: "src".into(),
            is_dir: true,
        };
        let paste = Intent::PasteFiles { dir: "src".into() };
        assert_eq!(FileKey::Copy.step(&rows(), Some("src/a.rs")), Some(copy));
        assert_eq!(FileKey::Delete.step(&rows(), Some("src")), Some(delete));
        assert_eq!(FileKey::Paste.step(&rows(), Some("src/a.rs")), Some(paste));
        assert_eq!(FileKey::Down.step(&[], None), None);
    }
}
