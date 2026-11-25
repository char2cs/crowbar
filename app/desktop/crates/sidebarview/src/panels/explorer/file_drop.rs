//! Dropping files from the operating system onto the Files panel.

use std::path::PathBuf;

use gpui::{App, ExternalPaths, Window};
use sidebar::{Command, FileKind, Intent, PathText};

use crate::panels::explorer::file_tree_entry::FileTreeEntry;
use crate::sidebar::{RowAction, RowHandler};

pub(crate) fn drop_directory(entry: &FileTreeEntry) -> String {
    match entry.kind {
        FileKind::Directory(_) => entry.path.clone(),
        FileKind::File => PathText::new(&entry.path).parent().to_owned(),
    }
}

pub(crate) fn upload_action(
    directory: &str,
    files: &[PathBuf],
) -> Option<RowAction> {
    if files.is_empty() {
        return None;
    }
    let command = Command::UploadPaths {
        path: directory.to_owned(),
        files: files.to_vec(),
    };
    Some(RowAction::Intent(Intent::Send(command)))
}

pub(crate) fn on_drop(
    handler: &RowHandler,
    directory: String,
) -> impl Fn(&ExternalPaths, &mut Window, &mut App) + 'static {
    let handler = std::rc::Rc::clone(handler);
    move |dropped, window, cx| {
        cx.stop_propagation();
        deliver(
            &handler,
            upload_action(&directory, dropped.paths()),
            window,
            cx,
        );
    }
}

fn deliver(
    handler: &RowHandler,
    action: Option<RowAction>,
    window: &mut Window,
    cx: &mut App,
) {
    let Some(action) = action else {
        return;
    };
    handler(action, window, cx);
}

#[cfg(test)]
mod tests {
    use gpui::{FileDropEvent, point, px};

    use super::*;
    use sidebar::{Expansion, GitStatus};

    use crate::panels::explorer::file_tree_row::FileTreeRow;
    use crate::panels::explorer::file_tree_row::TreeLook;
    use crate::test_support::{mount, recording_handler};

    fn entry(
        kind: FileKind,
        path: &str,
    ) -> FileTreeEntry {
        FileTreeEntry {
            path: path.into(),
            name: path.into(),
            depth: 0,
            kind,
            status: GitStatus::Clean,
            counts: None,
        }
    }

    #[test]
    fn a_drop_lands_in_the_folder_or_beside_the_file() {
        const CASES: &[(FileKind, &str, &str)] = &[
            (FileKind::Directory(Expansion::Collapsed), "src", "src"),
            (FileKind::File, "src/main.rs", "src"),
            (FileKind::File, "README.md", ""),
        ];
        for (kind, path, want) in CASES {
            assert_eq!(drop_directory(&entry(*kind, path)), *want, "{path}");
        }
    }

    #[test]
    fn no_files_means_no_upload() {
        assert_eq!(upload_action("src", &[]), None);
    }

    #[gpui::test]
    fn files_dropped_on_a_row_are_uploaded_into_its_folder(cx: &mut gpui::TestAppContext) {
        let (handler, seen) = recording_handler();
        let cx = mount(cx, move |_, _| FileTreeRow {
            entry: entry(FileKind::File, "src/main.rs"),
            neighbours: (0, 0),
            input: None,
            selected: false,
            cut: false,
            look: TreeLook::default(),
            scope: std::rc::Rc::default(),
            handler: std::rc::Rc::clone(&handler),
        });
        let at = point(px(40.0), px(10.0));
        let files = vec![PathBuf::from("/tmp/a.png")];
        cx.simulate_event(FileDropEvent::Entered {
            position: at,
            paths: ExternalPaths(files.iter().cloned().collect()),
        });
        cx.simulate_event(FileDropEvent::Submit { position: at });
        assert_eq!(
            seen.borrow().last(),
            upload_action("src", &files).as_ref(),
            "{:?}",
            seen.borrow()
        );
    }
}
