use std::collections::BTreeMap;

use crate::live::held::entities::Entities;
use crate::live::held::host_spec::HostSpec;
use crate::live::held::link_state::LinkState;

#[derive(Debug)]
pub(crate) struct HostSlot {
    pub(crate) spec: HostSpec,
    pub(crate) client: daemonapi::Client,
    pub(crate) store: Entities,
    pub(crate) link: LinkState,
    pub(crate) terminal_threads: bool,
    pub(crate) provider_icons: BTreeMap<String, String>,
    pub(crate) snapshot_failures: u32,
}

impl HostSlot {
    pub(crate) fn new(
        spec: HostSpec,
        client: daemonapi::Client,
    ) -> Self {
        Self {
            spec,
            client,
            store: Entities::default(),
            link: LinkState::default(),
            terminal_threads: false,
            provider_icons: BTreeMap::new(),
            snapshot_failures: 0,
        }
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::test_support::{FakeDaemon, local};

    #[test]
    fn a_new_slot_is_connecting_with_nothing_heard_and_no_terminal_agent() {
        let client = daemonapi::Client::new(FakeDaemon::shared(), local());
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
