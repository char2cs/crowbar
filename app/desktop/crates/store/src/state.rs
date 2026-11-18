//! The shared state the sidebar reads and the stream and commands write.

use std::collections::BTreeMap;

use crate::daemon_client::DaemonClient;
use crate::frame::Frame;
use crate::host_slot::HostSlot;
use crate::icon_cache::IconCache;
use crate::link_state::LinkState;
use crate::panel_state::PanelState;
use crate::pending_row::PendingRow;
use crate::settings_slot::SettingsSlot;
use crate::switch::Switch;

#[derive(Debug)]
pub(crate) struct State {
    pub(crate) hosts: Vec<HostSlot>,
    pub(crate) selected_host: Option<String>,
    pub(crate) active_project: Option<String>,
    pub(crate) local_rows: Vec<PendingRow>,
    pub(crate) panels: PanelState,
    pub(crate) unrevoked: BTreeMap<String, String>,
    pub(crate) icons: IconCache,
    pub(crate) settings: SettingsSlot,
}

impl State {
    pub(crate) fn new(hosts: Vec<HostSlot>) -> Self {
        Self {
            hosts,
            selected_host: None,
            active_project: None,
            local_rows: Vec::new(),
            panels: PanelState::default(),
            unrevoked: BTreeMap::new(),
            icons: IconCache::default(),
            settings: SettingsSlot::default(),
        }
    }

    pub(crate) fn add_host(
        &mut self,
        slot: HostSlot,
    ) {
        self.hosts.push(slot);
    }

    pub(crate) fn remove_host(
        &mut self,
        id: &str,
    ) {
        let owned_project = self
            .active_project
            .as_deref()
            .and_then(|project| {
                self.hosts
                    .iter()
                    .find(|slot| slot.store.projects.get(project).is_some())
            })
            .is_some_and(|slot| slot.spec.id == id);
        self.hosts.retain(|slot| slot.spec.id != id);
        self.unrevoked.remove(id);
        if self.selected_host.as_deref() == Some(id) {
            self.selected_host = None;
        }
        if owned_project {
            self.active_project = None;
        }
    }

    pub(crate) fn client_of_chat(
        &self,
        chat: &str,
    ) -> Option<DaemonClient> {
        self.hosts
            .iter()
            .find(|slot| slot.store.chats.get(chat).is_some())
            .map(|slot| slot.client.clone())
    }

    pub(crate) fn local_client(&self) -> Option<DaemonClient> {
        self.hosts
            .iter()
            .find(|slot| slot.spec.endpoint.is_none())
            .map(|slot| slot.client.clone())
    }

    pub(crate) fn host_mut(
        &mut self,
        id: &str,
    ) -> Option<&mut HostSlot> {
        self.hosts.iter_mut().find(|slot| slot.spec.id == id)
    }

    pub(crate) fn apply(
        &mut self,
        host: &str,
        frame: Frame,
    ) {
        if let Some(slot) = self.host_mut(host) {
            slot.store.apply(frame);
        }
    }

    pub(crate) fn set_terminal_threads(
        &mut self,
        host: &str,
        terminal: Switch,
    ) {
        if let Some(slot) = self.host_mut(host) {
            slot.terminal_threads = terminal.is_on();
        }
    }

    pub(crate) fn begin_snapshot(
        &mut self,
        host: &str,
    ) {
        if let Some(slot) = self.host_mut(host) {
            slot.store.begin_snapshot();
        }
    }

    pub(crate) fn link(
        &mut self,
        host: &str,
        link: LinkState,
    ) {
        if let Some(slot) = self.host_mut(host) {
            slot.link = link;
        }
    }
}

#[cfg(test)]
mod tests {
    use crate::test_support::state;

    #[test]
    fn frames_and_links_reach_the_named_host_only() {
        let (mut state, _) = state();
        state.link("local", crate::link_state::LinkState::Down);
        assert_eq!(state.hosts[0].link, crate::link_state::LinkState::Down);
        state.link("elsewhere", crate::link_state::LinkState::Live);
        state.begin_snapshot("local");
        assert!(state.host_mut("local").is_some());
    }
}
