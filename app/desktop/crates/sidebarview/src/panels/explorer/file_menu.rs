//! The context menu of a Files row.

use gpui::{ClipboardItem, SharedString};
use sidebar::{ClipMode, Command, FileAction, FileKind, Intent, MenuScope, NewEntry, PathText};
use ui::{IconName, MenuAction, MenuItem};

use crate::panels::explorer::file_tree_entry::FileTreeEntry;
use crate::panels::explorer::menu_lines::{FileEntry, MenuLines};
use crate::sidebar::{RowAction, RowHandler};

const ENV_TEMPLATES: [(&str, &str); 3] = [
    ("Create .env.example", ".env.example"),
    ("Create .env.local", ".env.local"),
    ("Create .env.development", ".env.development"),
];

fn directory_items(
    lines: &mut MenuLines,
    (entry, scope): (&FileTreeEntry, &MenuScope),
) {
    let path = || entry.path.clone();
    if !scope.is_locked() {
        let create = |entry| Intent::StartFileCreate { dir: path(), entry };
        lines.run(("New File", IconName::FilePlus), create(NewEntry::File));
        lines.run(
            ("New Folder", IconName::FolderPlus),
            create(NewEntry::Folder),
        );
        let upload = Command::UploadFile { path: path() };
        lines.send(("Upload File", IconName::UploadSimple), upload);
    }
    let refresh = Command::RefreshDirectory { path: path() };
    lines.send(("Refresh", IconName::ArrowClockwise), refresh);
    let open_all = Command::OpenAllFiles { path: path() };
    lines.send(("Open All Files", IconName::FolderOpen), open_all);
    let collapse = Command::CollapseDirectory { path: path() };
    lines.send(("Collapse All", IconName::CaretDoubleUp), collapse);
    let terminal = Command::OpenTerminalAt { path: path() };
    lines.send(("Open in Terminal", IconName::TerminalWindow), terminal);
    if scope.can_generate_image {
        let generate = Command::GenerateImage { path: path() };
        lines.send(("Generate Image", IconName::Image), generate);
    }
    if entry.path.is_empty() {
        let add = Command::AddFolderToWorkspace;
        lines.send(("Add Folder to Workspace", IconName::FolderSimplePlus), add);
    }
    lines.rule();
}

fn is_env_file(name: &str) -> bool {
    let name = name.trim().to_lowercase();
    name == ".env" || name.starts_with(".env.")
}

fn env_templates(
    lines: &mut MenuLines,
    path: &str,
) {
    lines.rule();
    for (label, target) in ENV_TEMPLATES {
        lines.send((label, IconName::FilePlus), env_template(path, target));
    }
}

fn env_template(
    path: &str,
    target: &str,
) -> Command {
    Command::CreateEnvTemplate {
        path: path.to_owned(),
        target: target.to_owned(),
    }
}

fn leaf_items(
    lines: &mut MenuLines,
    (entry, scope): (&FileTreeEntry, &MenuScope),
) {
    let path = || entry.path.clone();
    lines.send(("Open", IconName::FolderOpen), Command::OpenFile(path()));
    let content = Command::CopyFileContent { path: path() };
    lines.send(("Copy Content", IconName::Copy), content);
    if !scope.is_locked() && is_env_file(PathText::new(&entry.path).base()) {
        env_templates(lines, &entry.path);
    }
    lines.run(
        ("Properties", IconName::Info),
        Intent::ShowProperties(path()),
    );
    lines.rule();
}

fn relative(path: &str) -> String {
    if path.is_empty() {
        return ".".to_owned();
    }
    path.to_owned()
}

fn shared_items(
    lines: &mut MenuLines,
    (entry, scope): (&FileTreeEntry, &MenuScope),
) {
    lines.copy(
        ("Copy Path", IconName::Link),
        PathText::new(&entry.path).absolute(scope.root.as_deref()),
    );
    lines.copy(
        ("Copy Relative Path", IconName::FileText),
        relative(&entry.path),
    );
    let reveal = Command::RevealFile {
        path: entry.path.clone(),
    };
    if scope.local {
        lines.send(("Reveal in Finder", IconName::FolderOpen), reveal);
    }
    staging_items(lines, (entry, scope));
}

fn staging_items(
    lines: &mut MenuLines,
    (entry, scope): (&FileTreeEntry, &MenuScope),
) {
    let stage = |mode| Intent::StageFile {
        path: entry.path.clone(),
        mode,
    };
    lines.run(("Copy", IconName::Copy), stage(ClipMode::Copy));
    if !scope.is_locked() {
        let cut = ("Cut", IconName::Scissors);
        lines.run(cut, stage(ClipMode::Cut));
    }
    let remove = Command::RemoveFolderFromWorkspace {
        path: entry.path.clone(),
    };
    if scope.answers.can_remove_folder(&entry.path) {
        lines.send(("Remove From Workspace", IconName::FolderMinus), remove);
    }
}

fn management_items(
    lines: &mut MenuLines,
    entry: &FileTreeEntry,
) {
    let path = || entry.path.clone();
    let is_dir = matches!(entry.kind, FileKind::Directory(_));
    lines.rule();
    let duplicate = Command::DuplicateFile { path: path() };
    lines.send(("Duplicate", IconName::CopySimple), duplicate);
    let rename = Intent::StartFileRename(path());
    lines.run(("Rename", IconName::PencilSimple), rename);
    lines.rule();
    let delete = Intent::RequestFileDelete {
        path: path(),
        is_dir,
    };
    lines.run(("Delete", IconName::Trash), delete);
}

pub(crate) fn file_items(
    entry: &FileTreeEntry,
    scope: &MenuScope,
) -> Vec<Option<FileEntry>> {
    let mut lines = MenuLines::default();
    match entry.kind {
        FileKind::Directory(_) => directory_items(&mut lines, (entry, scope)),
        FileKind::File => leaf_items(&mut lines, (entry, scope)),
    }
    shared_items(&mut lines, (entry, scope));
    if !entry.path.is_empty() && !scope.is_locked() {
        management_items(&mut lines, entry);
    }
    lines.retain_served(&scope.answers);
    lines.0
}

pub(crate) fn file_menu(
    entry: &FileTreeEntry,
    scope: &MenuScope,
    handler: &RowHandler,
) -> Vec<MenuItem> {
    file_items(entry, scope)
        .into_iter()
        .map(|item| match item {
            Some(file_entry) => menu_row(file_entry, handler),
            None => MenuItem::Separator,
        })
        .collect()
}

fn menu_row(
    (label, icon, action): FileEntry,
    handler: &RowHandler,
) -> MenuItem {
    let handler = std::rc::Rc::clone(handler);
    let destructive = label == "Delete";
    let row = MenuAction::new(SharedString::from(label), move |window, cx| match &action {
        FileAction::Do(intent) => handler(RowAction::Intent(intent.clone()), window, cx),
        FileAction::CopyText(text) => {
            cx.write_to_clipboard(ClipboardItem::new_string(text.clone()));
        },
    })
    .icon(icon);
    if destructive {
        return MenuItem::Action(row.destructive());
    }
    MenuItem::Action(row)
}

#[cfg(test)]
mod tests {
    use super::*;
    use sidebar::{Command as Cmd, DemoShell, Expansion, GitStatus, Lock, ShellAnswers};
    use std::rc::Rc;

    fn entry(
        kind: FileKind,
        path: &str,
    ) -> FileTreeEntry {
        FileTreeEntry {
            path: path.into(),
            name: PathText::new(path).base().into(),
            depth: 1,
            kind,
            status: GitStatus::Clean,
            counts: None,
        }
    }

    fn scope() -> MenuScope {
        MenuScope {
            root: Some("/work".into()),
            lock: Lock::Unlocked,
            local: true,
            can_generate_image: false,
            answers: ShellAnswers::default(),
        }
    }

    fn labels(
        entry: &FileTreeEntry,
        scope: &MenuScope,
    ) -> Vec<&'static str> {
        file_items(entry, scope)
            .iter()
            .map(|item| item.as_ref().map_or("-", |(label, _, _)| *label))
            .collect()
    }

    fn directory(path: &str) -> FileTreeEntry {
        entry(FileKind::Directory(Expansion::Collapsed), path)
    }

    #[test]
    fn a_file_lists_open_copy_properties_paths_and_management() {
        assert_eq!(
            labels(&entry(FileKind::File, "src/a.rs"), &scope()),
            [
                "Open",
                "Copy Content",
                "Properties",
                "-",
                "Copy Path",
                "Copy Relative Path",
                "Reveal in Finder",
                "Copy",
                "Cut",
                "-",
                "Duplicate",
                "Rename",
                "-",
                "Delete",
            ]
        );
    }

    #[test]
    fn a_directory_lists_creation_navigation_paths_and_management() {
        assert_eq!(
            labels(&directory("src"), &scope()),
            [
                "New File",
                "New Folder",
                "Upload File",
                "Refresh",
                "Open All Files",
                "Collapse All",
                "Open in Terminal",
                "-",
                "Copy Path",
                "Copy Relative Path",
                "Reveal in Finder",
                "Copy",
                "Cut",
                "-",
                "Duplicate",
                "Rename",
                "-",
                "Delete",
            ]
        );
    }

    #[test]
    fn the_workspace_root_adds_the_folder_picker_and_drops_management() {
        let root = labels(&directory(""), &scope());
        assert!(root.contains(&"Add Folder to Workspace"));
        assert!(!root.contains(&"Rename") && !root.contains(&"Delete"));
        assert!(!labels(&directory("src"), &scope()).contains(&"Add Folder to Workspace"));
    }

    #[test]
    fn a_locked_workspace_keeps_only_the_entries_that_read() {
        let locked = MenuScope {
            lock: Lock::Locked,
            ..scope()
        };
        assert_eq!(
            labels(&directory("src"), &locked),
            [
                "Refresh",
                "Open All Files",
                "Collapse All",
                "Open in Terminal",
                "-",
                "Copy Path",
                "Copy Relative Path",
                "Reveal in Finder",
                "Copy",
            ]
        );
    }

    #[test]
    fn a_remote_workspace_has_no_file_manager_and_a_handler_adds_image_generation() {
        let remote = MenuScope {
            local: false,
            can_generate_image: true,
            ..scope()
        };
        let items = labels(&directory("src"), &remote);
        assert!(!items.contains(&"Reveal in Finder"));
        assert!(items.contains(&"Generate Image"));
    }

    #[test]
    fn an_env_file_offers_the_three_templates_unless_locked() {
        let env = entry(FileKind::File, "app/.env");
        let items = labels(&env, &scope());
        for label in [
            "Create .env.example",
            "Create .env.local",
            "Create .env.development",
        ] {
            assert!(items.contains(&label), "{label}");
        }
        let locked = MenuScope {
            lock: Lock::Locked,
            ..scope()
        };
        assert!(!labels(&env, &locked).contains(&"Create .env.local"));
        assert!(!labels(&entry(FileKind::File, "a.rs"), &scope()).contains(&"Create .env.local"));
    }

    fn action_of(
        entry: &FileTreeEntry,
        label: &str,
    ) -> Option<FileAction> {
        file_items(entry, &scope())
            .into_iter()
            .flatten()
            .find(|(name, _, _)| *name == label)
            .map(|(_, _, action)| action)
    }

    #[test]
    fn entries_carry_the_path_and_the_kind_of_the_row() {
        let file = entry(FileKind::File, "src/a.rs");
        let cases = [
            ("Copy Path", FileAction::CopyText("/work/src/a.rs".into())),
            (
                "Copy Relative Path",
                FileAction::CopyText("src/a.rs".into()),
            ),
            (
                "Delete",
                FileAction::Do(Intent::RequestFileDelete {
                    path: "src/a.rs".into(),
                    is_dir: false,
                }),
            ),
            (
                "Properties",
                FileAction::Do(Intent::ShowProperties("src/a.rs".into())),
            ),
            (
                "Reveal in Finder",
                FileAction::Do(Intent::Send(Command::RevealFile {
                    path: "src/a.rs".into(),
                })),
            ),
        ];
        for (label, want) in cases {
            assert_eq!(action_of(&file, label), Some(want), "{label}");
        }
        assert_eq!(
            action_of(&directory("src"), "Delete"),
            Some(FileAction::Do(Intent::RequestFileDelete {
                path: "src".into(),
                is_dir: true,
            }))
        );
    }

    #[test]
    fn the_root_copies_its_own_folder_and_a_dot() {
        let root = directory("");
        assert_eq!(
            action_of(&root, "Copy Path"),
            Some(FileAction::CopyText("/work".into()))
        );
        assert_eq!(
            action_of(&root, "Copy Relative Path"),
            Some(FileAction::CopyText(".".into()))
        );
    }

    fn asked(data: DemoShell) -> MenuScope {
        let data: Rc<dyn sidebar::Source> = Rc::new(data);
        MenuScope {
            answers: ShellAnswers::of(&data),
            ..scope()
        }
    }

    #[test]
    fn entries_the_shell_does_not_serve_are_hidden_with_their_separators() {
        let refresh = Cmd::RefreshDirectory { path: "src".into() };
        let declined = asked(DemoShell::reference().refusing(refresh));
        let items = labels(&directory("src"), &declined);
        assert!(!items.contains(&"Refresh"));
        assert!(items.contains(&"Open All Files"));
        let create = Cmd::CreateFile {
            path: String::new(),
        };
        let read_only = asked(DemoShell::reference().refusing(create));
        assert!(!labels(&directory("src"), &read_only).contains(&"New File"));
        assert_eq!(
            labels(&directory("src"), &asked(DemoShell::reference()))[0],
            "New File"
        );
    }

    #[test]
    fn a_folder_of_a_multi_folder_workspace_can_be_removed_from_it() {
        let several = asked(DemoShell::reference().with_removable_folder("src"));
        assert!(labels(&directory("src"), &several).contains(&"Remove From Workspace"));
        assert!(!labels(&directory("src"), &scope()).contains(&"Remove From Workspace"));
    }
}
