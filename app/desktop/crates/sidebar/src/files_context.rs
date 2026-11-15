//! What the Files panel needs besides the rows to draw them: the menu scope and the open input.

use std::rc::Rc;

use crate::editing::Editing;
use crate::expansion::Expansion;
use crate::file_kind::FileKind;
use crate::file_row::FileRow;
use crate::file_tree_entry::FileTreeEntry;
use crate::file_tree_row::FileTreeRow;
use crate::git_status::GitStatus;
use crate::menu_scope::MenuScope;
use crate::new_entry::NewEntry;
use crate::row_action::RowHandler;
use crate::row_input::RowInput;

pub(crate) struct FilesContext {
    pub(crate) handler: RowHandler,
    pub(crate) scope: Rc<MenuScope>,
    pub(crate) editing: Editing,
    pub(crate) input: Option<RowInput>,
    pub(crate) selected: Option<String>,
}

fn entry(row: FileRow) -> FileTreeEntry {
    FileTreeEntry {
        path: row.path,
        name: row.name,
        depth: row.depth,
        kind: row.kind,
        status: row.status,
        counts: None,
    }
}

impl FilesContext {
    fn row(
        &self,
        entry: FileTreeEntry,
        input: Option<RowInput>,
    ) -> FileTreeRow {
        FileTreeRow {
            selected: self.selected.as_deref() == Some(entry.path.as_str()),
            entry,
            input,
            scope: Rc::clone(&self.scope),
            handler: Rc::clone(&self.handler),
        }
    }

    fn rename_input(
        &self,
        path: &str,
    ) -> Option<RowInput> {
        match &self.editing {
            Editing::RenamingFile(target) if target == path => self.input.clone(),
            _ => None,
        }
    }

    fn draft_in(
        &self,
        (dir, depth): (&str, u8),
    ) -> Option<FileTreeRow> {
        let Editing::Creating {
            dir: target,
            entry: new,
        } = &self.editing
        else {
            return None;
        };
        if target != dir {
            return None;
        }
        let kind = match new {
            NewEntry::File => FileKind::File,
            NewEntry::Folder => FileKind::Directory(Expansion::Collapsed),
        };
        let draft = FileTreeEntry {
            path: format!("draft:{dir}"),
            name: String::new(),
            depth,
            kind,
            status: GitStatus::Clean,
            counts: None,
        };
        Some(self.row(draft, self.input.clone()))
    }

    pub(crate) fn rows(
        &self,
        files: Vec<FileRow>,
    ) -> Vec<FileTreeRow> {
        let mut rows: Vec<FileTreeRow> = self.draft_in(("", 0)).into_iter().collect();
        for file in files {
            let (path, below) = (file.path.clone(), file.depth.saturating_add(1));
            let input = self.rename_input(&path);
            rows.push(self.row(entry(file), input));
            rows.extend(self.draft_in((&path, below)));
        }
        rows
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn file(
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

    fn context(editing: Editing) -> FilesContext {
        FilesContext {
            handler: Rc::new(|_, _, _| {}),
            scope: Rc::default(),
            editing,
            input: None,
            selected: None,
        }
    }

    fn files() -> Vec<FileRow> {
        vec![
            file("src", 0, FileKind::Directory(Expansion::Expanded)),
            file("src/a.rs", 1, FileKind::File),
        ]
    }

    fn paths(rows: &[FileTreeRow]) -> Vec<&str> {
        rows.iter().map(|row| row.entry.path.as_str()).collect()
    }

    #[test]
    fn a_new_entry_row_is_placed_where_its_directory_says() {
        type Case = (&'static str, Editing, &'static [&'static str]);
        let creating = |dir: &str, entry| Editing::Creating {
            dir: dir.into(),
            entry,
        };
        let cases: [Case; 3] = [
            (
                "below its directory",
                creating("src", NewEntry::File),
                &["src", "draft:src", "src/a.rs"],
            ),
            (
                "first at the root",
                creating("", NewEntry::Folder),
                &["draft:", "src", "src/a.rs"],
            ),
            (
                "none while renaming",
                Editing::RenamingFile("src/a.rs".into()),
                &["src", "src/a.rs"],
            ),
        ];
        for (name, editing, want) in cases {
            let rows = context(editing).rows(files());
            assert_eq!(paths(&rows), want, "{name}");
        }
    }

    #[test]
    fn a_new_entry_nests_one_level_below_its_directory() {
        let creating = Editing::Creating {
            dir: "src".into(),
            entry: NewEntry::File,
        };
        let rows = context(creating).rows(files());
        assert_eq!(rows[1].entry.depth, 1);
    }
}
