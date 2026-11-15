//! The sidebar's read side over the shared state.

use std::sync::Arc;

use sidebar::{
    Changes, FileRow, Focus, Host, MoveTarget, Project, ProjectId, ProjectTree, RowId, ScopeSync,
    SidebarData,
};

use crate::hub::Hub;
use crate::sidebar_reads as reads;

/// What `sidebar::SidebarData` reads, answered from the entities the streams delivered.
#[derive(Debug, Clone)]
pub struct SidebarView {
    hub: Arc<Hub>,
}

impl SidebarView {
    pub(crate) fn new(hub: Arc<Hub>) -> Self {
        Self { hub }
    }
}

impl SidebarData for SidebarView {
    fn sync(&self) -> ScopeSync {
        self.hub.read(reads::sync)
    }

    fn hosts(&self) -> Vec<Host> {
        self.hub.read(reads::hosts)
    }

    fn projects(&self) -> Vec<Project> {
        self.hub.read(reads::projects)
    }

    fn active_project(&self) -> Option<ProjectId> {
        self.hub.read(reads::active_project)
    }

    fn tree(
        &self,
        project: &ProjectId,
    ) -> ProjectTree {
        self.hub.read(|state| reads::tree(state, project))
    }

    fn focus(&self) -> Focus {
        self.hub.read(reads::focus)
    }

    fn move_targets(
        &self,
        row: &RowId,
    ) -> Vec<MoveTarget> {
        self.hub.read(|state| reads::move_targets(state, row))
    }

    fn can_start_terminal_thread(&self) -> bool {
        self.hub
            .read(|state| reads::shown(state).is_some_and(|slot| slot.terminal_threads))
    }

    fn files(&self) -> Vec<FileRow> {
        self.hub.read(reads::files)
    }

    fn changes(&self) -> Changes {
        self.hub.read(reads::changes)
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::state;

    #[test]
    fn the_view_answers_from_the_shared_state() {
        let (state, _) = state();
        let view = SidebarView::new(Hub::new(state, Arc::new(|| {})));
        assert_eq!(view.sync(), ScopeSync::Ready);
        assert_eq!(view.projects().len(), 1);
        assert!(!view.can_start_terminal_thread());
        assert_eq!(view.files().len(), 0);
        assert_eq!(view.changes(), Changes::default());
    }
}
