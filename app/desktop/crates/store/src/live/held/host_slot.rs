//! Everything the shell holds of one host.

use daemonapi::DaemonClient;

use crate::live::held::entity_store::EntityStore;
use crate::live::held::host_spec::HostSpec;
use crate::live::held::link_state::LinkState;

#[derive(Debug)]
pub(crate) struct HostSlot {
    pub(crate) spec: HostSpec,
    pub(crate) client: DaemonClient,
    pub(crate) store: EntityStore,
    pub(crate) link: LinkState,
    pub(crate) terminal_threads: bool,
}

impl HostSlot {
    pub(crate) fn new(
        spec: HostSpec,
        client: DaemonClient,
    ) -> Self {
        Self {
            spec,
            client,
            store: EntityStore::default(),
            link: LinkState::default(),
            terminal_threads: false,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{FakeDaemon, local};

    #[test]
    fn a_new_slot_is_connecting_with_nothing_heard_and_no_terminal_agent() {
        let client = DaemonClient::new(FakeDaemon::shared(), local());
        let spec = HostSpec {
            id: "local".into(),
            name: "L".into(),
            endpoint: None,
        };
        let slot = HostSlot::new(spec, client);
        assert_eq!(slot.link, LinkState::Connecting);
        assert!(!slot.store.synced());
        assert!(!slot.terminal_threads);
    }
}
