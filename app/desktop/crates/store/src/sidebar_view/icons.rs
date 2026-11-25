//! The running requests the sidebar reads.

use sidebar::{IconTarget, Requests};

use crate::sidebar_view::SidebarView;

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
    use std::sync::Arc;

    use sidebar::{ProjectId, RowId};

    use super::*;
    use crate::live::{Hub, SettingsSlot};
    use crate::test_support::state;

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
