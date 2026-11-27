use crate::shell::listing::file_row::FileRow;
use crate::shell::listing::file_tree_view::FileTreeView;

pub trait FilesSource {
    fn files(&self) -> Vec<FileRow>;
    fn workspace_root(&self) -> Option<String> {
        None
    }
    fn file_tree_view(&self) -> FileTreeView {
        FileTreeView::default()
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::DemoShell;

    #[test]
    fn the_demo_files_have_a_root() {
        let data = DemoShell::reference();
        assert!(data.files().len() > 1);
        assert_eq!(data.workspace_root().as_deref(), Some("/work/demo"));
        assert_eq!(data.file_tree_view(), FileTreeView::default());
    }
}
