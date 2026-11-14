use std::sync::Arc;

use protocol::{ExtensionId, HostId, PaneId, PaneKind, RelayScope};
use transport::{
    ExactOrigins, HostRegistry, MemoryTokenStore, Relay, RelayConfig, StreamAccess, Transport,
};

use crate::echo_daemon::EchoDaemon;

pub(crate) const CORE_ORIGIN: &str = "crowbar://app";

/// A relay in front of an echo daemon, with the shell's origins configured.
pub(crate) struct RelayRig {
    pub(crate) relay: Relay,
    pub(crate) daemon: EchoDaemon,
    pub(crate) transport: Arc<Transport>,
}

impl RelayRig {
    pub(crate) async fn start(tag: &str, config: RelayConfig) -> Self {
        let daemon = EchoDaemon::start(tag);
        let transport = Arc::new(Transport::new(
            Arc::new(daemon.socket.clone()),
            Arc::new(HostRegistry::in_memory()),
            Arc::new(MemoryTokenStore::default()),
        ));
        let origins = ExactOrigins::new(vec![CORE_ORIGIN.to_owned()], "crowbar-ext://{extension}");
        let relay = Relay::start(Arc::clone(&transport), Arc::new(origins), config)
            .await
            .expect("relay starts");
        Self {
            relay,
            daemon,
            transport,
        }
    }

    pub(crate) fn token(&self, pane: &str, kind: PaneKind, extension: Option<&str>) -> String {
        let scope = RelayScope {
            host: HostId::new("local").expect("host id"),
            pane: PaneId::new(pane).expect("pane id"),
            kind,
            extension: extension.map(|id| ExtensionId::new(id).expect("extension id")),
        };
        self.relay.issue(&scope).expect("token issued").token
    }

    pub(crate) fn core_token(&self, pane: &str) -> String {
        self.token(pane, PaneKind::Core, None)
    }
}
