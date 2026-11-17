//! What the Files panel draws.

use crate::file_row::FileRow;
use crate::file_tree_view::FileTreeView;

/// What the Files panel draws.
pub trait FilesSource {
    /// The visible rows of the Files panel.
    fn files(&self) -> Vec<FileRow>;
    /// The absolute folder the Files panel shows; without it Copy Path copies the relative path.
    fn workspace_root(&self) -> Option<String> {
        None
    }
    /// How the file-tree settings shape the Files and Changes panels; read on every render.
    fn file_tree_view(&self) -> FileTreeView {
        FileTreeView::default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::demo_data::DemoData;

    #[test]
    fn the_demo_files_have_a_root() {
        let data = DemoData::reference();
        assert!(data.files().len() > 1);
        assert_eq!(data.workspace_root().as_deref(), Some("/work/demo"));
        assert_eq!(data.file_tree_view(), FileTreeView::default());
    }
}
