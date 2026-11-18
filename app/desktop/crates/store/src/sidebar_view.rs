//! The sidebar's read side over the shared state.

use std::sync::Arc;

use sidebar::{
    ActiveSpace, Focus, Host, MoveTarget, PaneFocus, Project, ProjectId, ProjectTree, RowActions,
    RowId, ScopeSync, SpaceList,
};

use crate::hub::Hub;
use crate::settings_slot::SettingsSlot;
use crate::{files_reads, host_reads, sidebar_reads as reads, space_icons};

/// What `sidebar::SidebarData` reads, answered from the entities the streams delivered.
#[derive(Debug, Clone)]
pub struct SidebarView {
    pub(crate) hub: Arc<Hub>,
    pub(crate) settings: SettingsSlot,
}

impl SidebarView {
    pub(crate) fn new(
        hub: Arc<Hub>,
        settings: SettingsSlot,
    ) -> Self {
        Self { hub, settings }
    }
}

impl SpaceList for SidebarView {
    fn sync(&self) -> ScopeSync {
        self.hub.read(reads::sync)
    }

    fn hosts(&self) -> Vec<Host> {
        self.hub.read(host_reads::hosts)
    }

    fn projects(&self) -> Vec<Project> {
        self.hub.read(reads::projects)
    }
}

impl ActiveSpace for SidebarView {
    fn active_project(&self) -> Option<ProjectId> {
        self.hub.read(reads::active_project)
    }

    fn tree(
        &self,
        project: &ProjectId,
    ) -> ProjectTree {
        self.hub.read(|state| reads::tree(state, project))
    }

    fn space_icon(
        &self,
        project: &ProjectId,
    ) -> sidebar::SpaceIcon {
        self.hub.read(|state| space_icons::of(state, project))
    }
}

impl PaneFocus for SidebarView {
    fn focus(&self) -> Focus {
        self.hub.read(reads::focus)
    }

    fn running_agents(&self) -> Vec<sidebar::RunningAgent> {
        self.hub.read(crate::plan_agent::running)
    }

    fn can_start_terminal_thread(&self) -> bool {
        self.hub
            .read(|state| reads::shown(state).is_some_and(|slot| slot.terminal_threads))
    }
}

impl RowActions for SidebarView {
    fn move_targets(
        &self,
        row: &RowId,
    ) -> Vec<MoveTarget> {
        self.hub.read(|state| reads::move_targets(state, row))
    }

    fn serves(
        &self,
        command: &sidebar::Command,
    ) -> bool {
        files_reads::shown(command)
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{Changes, ChangesSource as _, FilesSource as _};

    use super::*;
    use crate::test_support::state;

    #[test]
    fn the_view_answers_from_the_shared_state() {
        let (state, _) = state();
        let view = SidebarView::new(Hub::new(state, Arc::new(|| {})), SettingsSlot::default());
        assert_eq!(view.sync(), ScopeSync::Ready);
        assert_eq!(view.projects().len(), 1);
        assert!(!view.can_start_terminal_thread());
        assert_eq!(view.files().len(), 0);
        assert_eq!(view.changes(), Changes::default());
        assert_eq!(view.file_tree_view(), sidebar::FileTreeView::default());
    }
}
