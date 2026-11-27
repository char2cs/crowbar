use std::sync::Arc;

use protocol::{ExtensionId, HostId, PaneId, PaneOrigin, RelayScope};
use transport::{
    ExactOrigins, HostRegistry, MemoryTokenStore, Relay, RelayConfig, StreamAccess, Transport,
};

use crate::echo_daemon::EchoDaemon;
use crate::memory_listener::{MemoryDialer, memory_listener};

pub(crate) const CORE_ORIGIN: &str = "crowbar://app";
const RELAY_PORT: u16 = 4343;

pub(crate) struct RelayRig {
    pub(crate) relay: Relay,
    pub(crate) dialer: MemoryDialer,
    pub(crate) daemon: EchoDaemon,
    pub(crate) transport: Arc<Transport>,
}

impl RelayRig {
    pub(crate) fn start(config: RelayConfig) -> Result<Self, Box<dyn std::error::Error>> {
        let daemon = EchoDaemon::start()?;
        let transport = Arc::new(Transport::new(
            Arc::new(daemon.socket.clone()),
            Arc::new(HostRegistry::in_memory()),
            Arc::new(MemoryTokenStore::default()),
        ));
        let origins = ExactOrigins::new(vec![CORE_ORIGIN.to_owned()], "crowbar-ext://{extension}");
        let (acceptor, dialer) = memory_listener(RELAY_PORT);
        let relay = Relay::start_on(acceptor, Arc::clone(&transport), Arc::new(origins), config)?;
        Ok(Self {
            relay,
            dialer,
            daemon,
            transport,
        })
    }

    pub(crate) fn token(
        &self,
        pane: &str,
        origin: PaneOrigin,
    ) -> Result<String, Box<dyn std::error::Error>> {
        let scope = RelayScope::new(HostId::new("local")?, PaneId::new(pane)?, origin);
        Ok(self.relay.issue(&scope)?.token().to_owned())
    }

    pub(crate) fn extension_token(
        &self,
        pane: &str,
        extension: &str,
    ) -> Result<String, Box<dyn std::error::Error>> {
        let id = ExtensionId::new(extension)?;
        self.token(pane, PaneOrigin::Extension(id))
    }

    pub(crate) fn core_token(
        &self,
        pane: &str,
    ) -> Result<String, Box<dyn std::error::Error>> {
        self.token(pane, PaneOrigin::Core)
    }
}
