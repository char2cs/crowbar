//! The Files panel.

use gpui::{AnyElement, Context, IntoElement, ParentElement, Styled, div, px};
use ui::ScrollArea;

use crate::file_row::FileRow;
use crate::file_tree_entry::FileTreeEntry;
use crate::file_tree_row::FileTreeRow;
use crate::panel_message::panel_message;
use crate::row_action::RowHandler;
use crate::sidebar::Sidebar;

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

fn rows(files: Vec<FileRow>, handler: &RowHandler) -> impl Iterator<Item = FileTreeRow> + '_ {
    files.into_iter().map(|row| FileTreeRow {
        entry: entry(row),
        handler: std::rc::Rc::clone(handler),
    })
}

impl Sidebar {
    pub(crate) fn files_panel(&mut self, cx: &mut Context<'_, Self>) -> AnyElement {
        let files = self.data.files();
        if files.is_empty() {
            return panel_message("No folder open", cx);
        }
        let handler = Self::row_handler(cx);
        let list = div()
            .flex()
            .flex_col()
            .py(px(4.0))
            .children(rows(files, &handler));
        ScrollArea::new("files-scroll", &self.files_scroll, list).into_any_element()
    }
}

#[cfg(test)]
#[path = "files_panel_tests.rs"]
mod tests;
