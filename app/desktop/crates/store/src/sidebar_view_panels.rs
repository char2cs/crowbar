//! The sidebar view's answers about the Files and Changes panels.

use sidebar::{Changes, ChangesSource, FileAccess, FileRow, FilesSource};

use crate::sidebar_view::SidebarView;
use crate::{files_reads, sidebar_reads as reads};

impl FilesSource for SidebarView {
    fn files(&self) -> Vec<FileRow> {
        self.hub.read(reads::files)
    }

    fn workspace_root(&self) -> Option<String> {
        self.hub.read(files_reads::root)
    }

    fn file_tree_view(&self) -> sidebar::FileTreeView {
        crate::file_tree_reads::view_of(|key| self.settings.get(key))
    }
}

impl FileAccess for SidebarView {
    fn files_locked(&self) -> bool {
        self.hub.read(files_reads::locked)
    }

    fn files_are_local(&self) -> bool {
        self.hub.read(files_reads::local)
    }
}

impl ChangesSource for SidebarView {
    fn changes(&self) -> Changes {
        self.hub.read(reads::changes)
    }

    fn merge_box(&self) -> Option<sidebar::MergeBox> {
        self.hub.read(crate::merge_reads::merge_box)
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::hub::Hub;
    use crate::settings_slot::SettingsSlot;
    use crate::test_support::state;

    #[test]
    fn with_no_workspace_open_the_panels_are_empty() {
        let (state, _) = state();
        let view = SidebarView::new(Hub::new(state, Arc::new(|| {})), SettingsSlot::default());
        assert_eq!(view.files().len(), 0);
        assert_eq!(view.workspace_root(), None);
        assert_eq!(view.changes().files.len(), 0);
    }
}
