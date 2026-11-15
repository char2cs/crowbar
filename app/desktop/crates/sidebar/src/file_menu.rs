//! The context menu of a Files or Changes row.

use gpui::{ClipboardItem, SharedString};
use ui::{IconName, MenuAction, MenuItem};

use crate::command::Command;
use crate::file_action::FileAction;
use crate::file_kind::FileKind;
use crate::file_tree_entry::FileTreeEntry;
use crate::intent::Intent;
use crate::row_action::{RowAction, RowHandler};

pub(crate) type FileEntry = (&'static str, IconName, FileAction);

pub(crate) fn file_actions(entry: &FileTreeEntry) -> Vec<FileEntry> {
    let open = (entry.kind == FileKind::File).then(|| {
        (
            "Open",
            IconName::Code,
            FileAction::Send(Command::OpenFile(entry.path.clone())),
        )
    });
    let copy = (
        "Copy Relative Path",
        IconName::Copy,
        FileAction::CopyPath(entry.path.clone()),
    );
    let reveal = (
        "Reveal in file manager",
        IconName::FolderOpen,
        FileAction::Send(Command::RevealFile {
            path: entry.path.clone(),
        }),
    );
    open.into_iter().chain([copy, reveal]).collect()
}

pub(crate) fn file_menu(
    entry: &FileTreeEntry,
    handler: &RowHandler,
) -> Vec<MenuItem> {
    let mut items: Vec<MenuItem> = file_actions(entry)
        .into_iter()
        .map(|file_entry| menu_row(file_entry, handler))
        .collect();
    if entry.kind == FileKind::File {
        items.insert(1, MenuItem::Separator);
    }
    items
}

fn menu_row(
    (label, icon, action): FileEntry,
    handler: &RowHandler,
) -> MenuItem {
    let handler = std::rc::Rc::clone(handler);
    let row = MenuAction::new(SharedString::from(label), move |window, cx| {
        perform(&action, &handler, (window, cx));
    });
    MenuItem::Action(row.icon(icon))
}

fn perform(
    action: &FileAction,
    handler: &RowHandler,
    (window, cx): (&mut gpui::Window, &mut gpui::App),
) {
    match action {
        FileAction::Send(command) => send(command, handler, (window, cx)),
        FileAction::CopyPath(path) => {
            cx.write_to_clipboard(ClipboardItem::new_string(path.clone()))
        },
    }
}

fn send(
    command: &Command,
    handler: &RowHandler,
    (window, cx): (&mut gpui::Window, &mut gpui::App),
) {
    let intent = RowAction::Intent(Intent::Send(command.clone()));
    handler(intent, window, cx);
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::expansion::Expansion;
    use crate::git_status::GitStatus;

    fn entry(kind: FileKind) -> FileTreeEntry {
        FileTreeEntry {
            path: "src/a.rs".into(),
            name: "a.rs".into(),
            depth: 1,
            kind,
            status: GitStatus::Clean,
            counts: None,
        }
    }

    #[test]
    fn a_file_offers_open_copy_and_reveal_and_a_directory_only_the_last_two() {
        type Case = (FileKind, &'static [&'static str]);
        const CASES: [Case; 2] = [
            (
                FileKind::File,
                &["Open", "Copy Relative Path", "Reveal in file manager"],
            ),
            (
                FileKind::Directory(Expansion::Collapsed),
                &["Copy Relative Path", "Reveal in file manager"],
            ),
        ];
        for (kind, want) in CASES {
            let labels: Vec<&str> = file_actions(&entry(kind))
                .iter()
                .map(|(label, _, _)| *label)
                .collect();
            assert_eq!(labels, want, "{kind:?}");
        }
    }

    #[test]
    fn reveal_asks_the_shell_to_show_the_row_path() {
        let actions = file_actions(&entry(FileKind::File));
        let reveal = actions.last().map(|(_, _, action)| action.clone());
        let want = FileAction::Send(Command::RevealFile {
            path: "src/a.rs".into(),
        });
        assert_eq!(reveal, Some(want));
    }
}
