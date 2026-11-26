//! The sidebar's read side over the shared state.

mod file_tree;
mod hosts;
mod panels;
mod space_list;

use std::sync::Arc;

use sidebar::{
    ActiveSpace, Focus, IconTarget, MoveTarget, PaneFocus, ProjectId, ProjectTree, Requests,
    RowActions, RowId,
};

use crate::live::Hub;
use crate::live::SettingsSlot;
use crate::{
    reads,
    reads::{files as files_reads, space_icons},
};

/// What `sidebar::Source` reads, answered from the entities the streams delivered.
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
        self.hub.read(crate::plan::running)
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

impl Requests for SidebarView {
    fn icon_busy(
        &self,
        target: &IconTarget,
    ) -> bool {
        self.hub.read(|state| state.marking.contains(target))
    }

    fn adding(
        &self,
        folder: &str,
    ) -> bool {
        self.hub.read(|state| state.adding.contains(folder))
    }
}

#[cfg(test)]
mod tests {
    use sidebar::{Changes, ChangesSource as _, FilesSource as _, ScopeSync, SpaceList as _};

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

    #[test]
    fn a_target_is_busy_while_the_state_marks_it() {
        let (mut state, _) = state();
        let busy = IconTarget::Repo(RowId::from("rdo"));
        state.marking.insert(busy.clone());
        let view = SidebarView::new(Hub::new(state, Arc::new(|| {})), SettingsSlot::default());
        assert!(view.icon_busy(&busy), "a request for the repo's icon runs");
        let space = IconTarget::Space(ProjectId::from("p"));
        assert!(!view.icon_busy(&space), "none runs for the space's");
    }
}
