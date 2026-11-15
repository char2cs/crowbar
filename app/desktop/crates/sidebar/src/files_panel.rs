//! The Files panel.

use gpui::{AnyElement, Context, IntoElement, ParentElement, Styled, div};
use ui::ScrollArea;

use crate::file_row::FileRow;
use crate::file_tree_entry::FileTreeEntry;
use crate::file_tree_row::FileTreeRow;
use crate::panel_message::panel_message;
use crate::row_action::RowHandler;
use crate::scale::SPACE_XS;
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

fn rows(
    files: Vec<FileRow>,
    handler: &RowHandler,
) -> impl Iterator<Item = FileTreeRow> + '_ {
    files.into_iter().map(|row| FileTreeRow {
        entry: entry(row),
        handler: std::rc::Rc::clone(handler),
    })
}

impl Sidebar {
    pub(crate) fn files_panel(
        &mut self,
        cx: &mut Context<'_, Self>,
    ) -> AnyElement {
        let files = self.data.files();
        if files.is_empty() {
            return panel_message("No folder open", cx);
        }
        let handler = Self::row_handler(cx);
        let list = div()
            .flex()
            .flex_col()
            .py(SPACE_XS)
            .children(rows(files, &handler));
        ScrollArea::new("files-scroll", &self.scrolls.files, list).into_any_element()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::prelude::*;

    #[gpui::test]
    fn the_files_panel_renders_the_workspace_files(cx: &mut TestAppContext) {
        let (_, sidebar, cx) = open(cx, DemoData::reference());
        sidebar.update(cx, |this, cx| this.set_mode(Mode::Files, cx));
        cx.run_until_parked();
        assert!(!sidebar.read_with(cx, |this, _| this.data.files().is_empty()));
    }

    #[test]
    fn a_directory_row_toggles_and_a_file_row_opens() {
        use crate::expansion::Expansion;
        use crate::file_kind::FileKind;
        use crate::file_row::FileRow;
        use crate::git_status::GitStatus;
        let row = |kind: FileKind| FileRow {
            path: "src".into(),
            name: "src".into(),
            depth: 0,
            kind,
            status: GitStatus::Clean,
        };
        assert_ne!(
            entry(row(FileKind::Directory(Expansion::Collapsed))).kind,
            entry(row(FileKind::File)).kind
        );
    }
}
