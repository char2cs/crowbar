//! The spaces and hosts the sidebar lists, as the state holds them.

use sidebar::{Host, HostChoice, HostId, Project, ScopeSync, SpaceList};

use crate::reads;
use crate::sidebar_view::SidebarView;
use crate::sidebar_view::hosts as host_reads;

impl SpaceList for SidebarView {
    fn sync(&self) -> ScopeSync {
        self.hub.read(reads::sync)
    }

    fn projects(&self) -> Vec<Project> {
        self.hub.read(reads::projects)
    }
}

impl HostChoice for SidebarView {
    fn hosts(&self) -> Vec<Host> {
        self.hub.read(host_reads::hosts)
    }

    fn shown_host(&self) -> Option<HostId> {
        self.hub.read(host_reads::shown_host)
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use sidebar::{HostChoice as _, HostId, SpaceList as _};

    use crate::live::{HostSlot, HostSpec, Hub, LinkState, SettingsSlot};
    use crate::sidebar_view::SidebarView;
    use crate::test_support::state;

    fn down_remote(client: daemonapi::Client) -> HostSlot {
        let spec = HostSpec {
            id: "box".into(),
            name: "box".into(),
            endpoint: Some("box:3737".into()),
        };
        let mut slot = HostSlot::new(spec, client);
        slot.link = LinkState::Down;
        slot
    }

    #[test]
    fn a_chosen_host_that_is_down_is_shown_though_it_lists_no_space() {
        let (mut state, _) = state();
        state.add_host(down_remote(state.hosts[0].client.clone()));
        state.selected_host = Some("box".into());
        let view = SidebarView::new(Hub::new(state, Arc::new(|| {})), SettingsSlot::default());
        let shown = (view.shown_host(), view.projects().len());
        assert_eq!(shown, (Some(HostId::from("box")), 0));
    }
}
