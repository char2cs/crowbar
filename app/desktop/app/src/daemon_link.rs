use std::path::PathBuf;
use std::sync::Arc;

use protocol::{DaemonTransport, HostId};
use supervisor::Supervisor;
use tokio::runtime::Runtime;
use tokio::sync::watch;
use transport::{
    CachedTokenStore, ExactOrigins, HostRegistry, OsKeychain, Relay, RelayConfig, ShellTransport,
    TokenStore, Transport,
};

use crate::app_options::AppOptions;
use crate::error::{Error, Result};
use crate::link_state::{LinkState, linked};
use crate::transport_book::TransportBook;

mod host_specs;
mod launch;
mod lifecycle;

const LOCAL_HOST: &str = "local";
const CORE_ORIGIN: &str = "crowbar://local";

pub(crate) struct DaemonLink {
    runtime: Runtime,
    options: AppOptions,
    state: Arc<LinkState>,
    transport: Arc<dyn DaemonTransport>,
    pub(crate) daemon: Arc<Transport>,
    registry: Arc<HostRegistry>,
    desk: store::HostDesk,
    phases: async_channel::Receiver<startup::Input>,
    sender: async_channel::Sender<startup::Input>,
    host: HostId,
}

impl std::fmt::Debug for DaemonLink {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("DaemonLink")
            .field("supervised", &self.supervisor().is_some())
            .finish_non_exhaustive()
    }
}

impl DaemonLink {
    pub(crate) fn start(options: &AppOptions) -> Result<Self> {
        let runtime = tokio::runtime::Builder::new_multi_thread()
            .worker_threads(2)
            .enable_all()
            .build()
            .map_err(|e| Error::Daemon(e.to_string()))?;
        let (sender, phases) = async_channel::unbounded();
        let (state, locator) = linked();
        let state = Arc::new(state);
        runtime.block_on(launch::try_launch(options, &state, &sender));
        let (transport, daemon, registry, tokens) =
            runtime.block_on(build_transport(options, locator))?;
        let desk = host_desk(options, &registry, tokens)?;
        Ok(Self {
            runtime,
            options: options.clone(),
            state,
            transport,
            daemon,
            registry,
            desk,
            phases,
            sender,
            host: HostId::new(LOCAL_HOST).map_err(|e| Error::Daemon(e.to_string()))?,
        })
    }

    pub(crate) fn transport(&self) -> Arc<dyn DaemonTransport> {
        Arc::clone(&self.transport)
    }

    pub(crate) fn local_host(&self) -> HostId {
        self.host.clone()
    }

    pub(crate) fn handle(&self) -> tokio::runtime::Handle {
        self.runtime.handle().clone()
    }

    pub(crate) fn desk(&self) -> store::HostDesk {
        self.desk.clone()
    }

    pub(crate) fn phases(&self) -> async_channel::Receiver<startup::Input> {
        self.phases.clone()
    }

    pub(crate) fn socket(&self) -> Option<PathBuf> {
        self.state.socket()
    }

    pub(crate) fn log(&self) -> Option<PathBuf> {
        self.state.log()
    }

    fn supervisor(&self) -> Option<Arc<Supervisor>> {
        self.state.supervisor()
    }
}

fn host_desk(
    options: &AppOptions,
    registry: &Arc<HostRegistry>,
    tokens: Arc<dyn TokenStore>,
) -> Result<store::HostDesk> {
    let notes = store::HostNotes::open(options.state_dir().join("host-notes.json"))
        .map_err(|e| Error::Daemon(e.to_string()))?;
    Ok(store::HostDesk {
        book: Arc::new(TransportBook::new(Arc::clone(registry), tokens)),
        notes: Arc::new(notes),
    })
}

async fn build_transport(
    options: &AppOptions,
    locator: watch::Receiver<Option<PathBuf>>,
) -> Result<(
    Arc<dyn DaemonTransport>,
    Arc<Transport>,
    Arc<HostRegistry>,
    Arc<dyn TokenStore>,
)> {
    let registry = Arc::new(
        HostRegistry::open(options.state_dir().join("hosts.json"))
            .map_err(|e| Error::Daemon(e.to_string()))?,
    );
    let tokens: Arc<dyn TokenStore> = Arc::new(CachedTokenStore::new(OsKeychain::desktop()));
    let transport = Arc::new(Transport::new(
        Arc::new(locator),
        Arc::clone(&registry) as _,
        Arc::clone(&tokens),
    ));
    let origins = ExactOrigins::new(vec![CORE_ORIGIN.to_owned()], CORE_ORIGIN);
    let daemon = Arc::clone(&transport);
    let relay = Relay::start(
        Arc::clone(&transport),
        Arc::new(origins),
        RelayConfig::default(),
    )
    .await
    .map_err(|e| Error::Daemon(e.to_string()))?;
    let shell = ShellTransport::new(
        tokio::runtime::Handle::current(),
        transport,
        Arc::new(relay),
    );
    Ok((Arc::new(shell), daemon, registry, tokens))
}

#[cfg(test)]
mod tests {
    use crate::test_support::link_without_daemon;

    #[test]
    fn a_link_without_a_daemon_reports_the_halt_and_has_nothing_to_shut_down() {
        let link = link_without_daemon();
        let phase = link.phases().recv_blocking().unwrap();
        assert!(matches!(phase, startup::Input::Halted { .. }), "{phase:?}");
        assert!(link.log().is_none(), "nothing to open");
        link.shutdown();
        link.shutdown();
        assert!(link.supervisor().is_none(), "nothing left to stop");
    }
}
