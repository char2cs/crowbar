//! Everything the shell holds of one host.

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
