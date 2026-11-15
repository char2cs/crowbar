use std::sync::Arc;

use protocol::{ExtensionId, HostId, PaneId, PaneOrigin, RelayScope};
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
    pub(crate) async fn start(config: RelayConfig) -> Self {
        let daemon = EchoDaemon::start();
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

    pub(crate) fn token(
        &self,
        pane: &str,
        origin: PaneOrigin,
    ) -> String {
        let scope = RelayScope::new(
            HostId::new("local").expect("host id"),
            PaneId::new(pane).expect("pane id"),
            origin,
        );
        self.relay
            .issue(&scope)
            .expect("token issued")
            .token()
            .to_owned()
    }

    pub(crate) fn extension_token(
        &self,
        pane: &str,
        extension: &str,
    ) -> String {
        let id = ExtensionId::new(extension).expect("extension id");
        self.token(pane, PaneOrigin::Extension(id))
    }

    pub(crate) fn core_token(
        &self,
        pane: &str,
    ) -> String {
        self.token(pane, PaneOrigin::Core)
    }
}
