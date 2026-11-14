use super::*;
use crate::test_support::prelude::*;

use crate::expansion::Expansion;
use crate::file_kind::FileKind;
use crate::git_status::GitStatus;
use crate::test_support::measure;

fn entry(kind: FileKind, path: &str) -> FileTreeEntry {
    FileTreeEntry {
        path: path.into(),
        name: path.into(),
        depth: 0,
        kind,
        status: GitStatus::Clean,
        counts: None,
    }
}

#[gpui::test]
fn a_row_is_twenty_six_pixels_tall(cx: &mut TestAppContext) {
    let bounds = measure(cx, |_, _| FileTreeRow {
        entry: entry(FileKind::File, "main.rs"),
        handler: std::rc::Rc::new(|_, _, _| {}),
    });
    assert_eq!(f32::from(bounds.size.height), ROW_HEIGHT);
}

#[test]
fn a_directory_toggles_and_a_file_opens() {
    assert_eq!(
        command(&entry(FileKind::Directory(Expansion::Collapsed), "src")),
        Command::ToggleDirectory("src".into())
    );
    assert_eq!(
        command(&entry(FileKind::File, "a.rs")),
        Command::OpenFile("a.rs".into())
    );
}
