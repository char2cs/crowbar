//! What the Files panel needs besides the rows to draw them: the menu scope and the open input.

use sidebar::{Editing, Expansion, FileKind, FileRow, GitStatus, MenuScope, NewEntry};
use std::rc::Rc;

use crate::panels::chats::RowInput;
use crate::panels::explorer::file_tree_entry::FileTreeEntry;
use crate::panels::explorer::file_tree_row::FileTreeRow;
use crate::panels::explorer::file_tree_row::TreeLook;
use crate::sidebar::RowHandler;

pub(crate) struct FilesContext {
    pub(crate) handler: RowHandler,
    pub(crate) scope: Rc<MenuScope>,
    pub(crate) editing: Editing,
    pub(crate) input: Option<RowInput>,
    pub(crate) selected: Option<String>,
    pub(crate) cut: Vec<String>,
    pub(crate) look: TreeLook,
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

pub(crate) fn neighbours(
    depths: &[u8],
    at: usize,
) -> (u8, u8) {
    let above = at.checked_sub(1).and_then(|above| depths.get(above));
    let below = depths.get(at.saturating_add(1));
    (above.copied().unwrap_or(0), below.copied().unwrap_or(0))
}

impl FilesContext {
    fn row(
        &self,
        entry: FileTreeEntry,
        input: Option<RowInput>,
    ) -> FileTreeRow {
        FileTreeRow {
            selected: self.selected.as_deref() == Some(entry.path.as_str()),
            cut: self.cut.contains(&entry.path),
            entry,
            neighbours: (0, 0),
            input,
            look: self.look.clone(),
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
        let depths: Vec<u8> = rows.iter().map(|row| row.entry.depth).collect();
        for (at, row) in rows.iter_mut().enumerate() {
            row.neighbours = neighbours(&depths, at);
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
            cut: Vec::new(),
            look: TreeLook::default(),
        }
    }

    #[test]
    fn a_file_staged_by_cut_is_drawn_cut_and_its_neighbours_are_not() {
        let staged = FilesContext {
            cut: vec!["src/a.rs".into()],
            ..context(Editing::default())
        };
        let rows = staged.rows(files());
        let cut: Vec<(&str, bool)> = rows
            .iter()
            .map(|row| (row.entry.path.as_str(), row.cut))
            .collect();
        assert_eq!(cut, [("src", false), ("src/a.rs", true)]);
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
    fn a_new_entry_nests_one_level_below_its_directory_between_its_neighbours() {
        let creating = Editing::Creating {
            dir: "src".into(),
            entry: NewEntry::File,
        };
        let rows = context(creating).rows(files());
        let placed: Vec<_> = rows
            .iter()
            .map(|row| (row.entry.depth, row.neighbours))
            .collect();
        assert_eq!(placed, [(0, (0, 1)), (1, (0, 1)), (1, (1, 0))]);
    }
}
