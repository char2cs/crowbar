//! The sidebar view's answers about the Files and Changes panels.

use sidebar::{
    Changes, ChangesSource, FileAccess, FileRow, FilesSource, FolderPhase, FolderSource,
};

use crate::sidebar_view::SidebarView;
use crate::{reads, reads::files as files_reads};

impl FilesSource for SidebarView {
    fn files(&self) -> Vec<FileRow> {
        self.hub.read(reads::files)
    }

    fn workspace_root(&self) -> Option<String> {
        self.hub.read(files_reads::root)
    }

    fn file_tree_view(&self) -> sidebar::FileTreeView {
        crate::sidebar_view::file_tree::view_of(|key| self.settings.get(key))
    }
}

impl FolderSource for SidebarView {
    fn folder_phase(&self) -> FolderPhase {
        self.hub.read(files_reads::phase)
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
        self.hub.read(reads::merge_box)
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::live::Hub;
    use crate::live::SettingsSlot;
    use crate::test_support::state;

    #[test]
    fn the_folder_loads_from_its_ask_until_its_root_is_listed() {
        let (state, _) = state();
        let hub = Hub::new(state, Arc::new(|| {}));
        let view = SidebarView::new(Arc::clone(&hub), SettingsSlot::default());
        let empty = (view.folder_phase(), view.workspace_root());
        assert_eq!(empty, (FolderPhase::NoFolder, None), "no workspace open");
        hub.edit(|state| state.panels.follow(Some("hc".into())));
        assert!(hub.edit(|state| state.panels.ask().is_some()));
        assert_eq!(view.folder_phase(), FolderPhase::Loading);
        hub.edit(|state| state.panels.list("hc", "", Vec::new()));
        assert_eq!(view.folder_phase(), FolderPhase::Listed);
    }
}
