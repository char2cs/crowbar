use std::path::PathBuf;
use std::sync::Arc;

use daemon_supervisor::{DaemonEvent, DaemonPaths, Supervisor, SupervisorConfig, locate_binary};
use protocol::{DaemonTransport, HostId};
use startup_view::StartupInput;
use tokio::runtime::Runtime;
use tokio::sync::watch;
use transport::{
    CachedTokenStore, ExactOrigins, HostRegistry, OsKeychain, Relay, RelayConfig, ShellTransport,
    TokenStore, Transport,
};

use crate::app_options::AppOptions;
use crate::error::{Error, Result};
use crate::link_state::{LinkState, linked};
use crate::phase_input::phase_input;
use crate::transport_book::TransportBook;

const LOCAL_HOST: &str = "local";
const CORE_ORIGIN: &str = "crowbar://local";

pub(crate) struct DaemonLink {
    runtime: Runtime,
    options: AppOptions,
    state: Arc<LinkState>,
    transport: Arc<dyn DaemonTransport>,
    registry: Arc<HostRegistry>,
    desk: shell_data::HostDesk,
    phases: async_channel::Receiver<StartupInput>,
    sender: async_channel::Sender<StartupInput>,
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
        runtime.block_on(try_launch(options, &state, &sender));
        let (transport, registry, tokens) = runtime.block_on(build_transport(options, locator))?;
        let desk = host_desk(options, &registry, tokens)?;
        Ok(Self {
            runtime,
            options: options.clone(),
            state,
            transport,
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

    pub(crate) fn desk(&self) -> shell_data::HostDesk {
        self.desk.clone()
    }

    fn name_of(
        &self,
        id: &str,
    ) -> String {
        self.desk
            .notes
            .get(id)
            .map_or_else(|| id.to_owned(), |note| note.name)
    }

    pub(crate) fn hosts(&self) -> Vec<shell_data::HostSpec> {
        let local = shell_data::HostSpec {
            id: "local".to_owned(),
            name: "This computer".to_owned(),
            endpoint: None,
        };
        let paired = self
            .registry
            .list()
            .into_iter()
            .map(|(id, endpoint)| shell_data::HostSpec {
                name: self.name_of(id.as_str()),
                id: id.as_str().to_owned(),
                endpoint: Some(String::from(endpoint)),
            });
        std::iter::once(local).chain(paired).collect()
    }

    pub(crate) fn phases(&self) -> async_channel::Receiver<StartupInput> {
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

    pub(crate) fn shutdown(&self) {
        let Some(supervisor) = self.state.take_supervisor() else {
            return;
        };
        match Arc::try_unwrap(supervisor) {
            Ok(owned) => self.runtime.block_on(owned.stop()),
            Err(_) => {
                tracing::warn!("a restart is still running; the daemon will be killed on exit");
            },
        }
    }

    pub(crate) fn restart(&self) {
        let Some(supervisor) = self.supervisor() else {
            return self.relaunch();
        };
        self.runtime.spawn(async move {
            supervisor
                .restart()
                .await
                .unwrap_or_else(|error| tracing::warn!(%error, "restarting the daemon failed"));
        });
    }

    fn relaunch(&self) {
        let options = self.options.clone();
        let state = Arc::clone(&self.state);
        let sender = self.sender.clone();
        self.runtime
            .spawn(async move { relaunch_unless_running(&options, &state, &sender).await });
    }
}

async fn relaunch_unless_running(
    options: &AppOptions,
    state: &LinkState,
    sender: &async_channel::Sender<StartupInput>,
) {
    let _turn = state.turn().await;
    if state.supervisor().is_none() {
        try_launch(options, state, sender).await;
    }
}

fn report_halt(
    sender: &async_channel::Sender<StartupInput>,
    error: &Error,
) {
    let halted = StartupInput::Halted {
        reason: None,
        message: error.to_string(),
    };
    if sender.try_send(halted).is_err() {
        tracing::warn!("the startup view is gone; the daemon failure goes unreported");
    }
}

async fn try_launch(
    options: &AppOptions,
    state: &LinkState,
    sender: &async_channel::Sender<StartupInput>,
) {
    match launch(options, sender.clone()).await {
        Ok((supervisor, paths)) => state.adopt(supervisor, paths),
        Err(error) => report_halt(sender, &error),
    }
}

async fn launch(
    options: &AppOptions,
    sender: async_channel::Sender<StartupInput>,
) -> Result<(Arc<Supervisor>, DaemonPaths)> {
    let current = std::env::current_exe().ok();
    let path_var = std::env::var_os("PATH");
    let binary = locate_binary(
        options.daemon_binary.as_deref(),
        current.as_deref(),
        path_var.as_deref(),
    )
    .map_err(|e| Error::Daemon(e.to_string()))?;
    let paths = DaemonPaths::resolve(&binary, Some(&options.home))
        .await
        .map_err(|e| Error::Daemon(e.to_string()))?;
    let config = SupervisorConfig::new(
        binary,
        paths.clone(),
        Some(options.home.clone()),
        env!("CARGO_PKG_VERSION"),
    );
    let supervisor = Arc::new(Supervisor::start(config));
    forward_phases(&supervisor, sender);
    Ok((supervisor, paths))
}

fn forward_phases(
    supervisor: &Supervisor,
    sender: async_channel::Sender<StartupInput>,
) {
    let mut current = supervisor.current();
    tokio::spawn(async move { while relay_phase(&sender, &mut current).await {} });
}

async fn relay_phase(
    sender: &async_channel::Sender<StartupInput>,
    current: &mut watch::Receiver<DaemonEvent>,
) -> bool {
    let input = phase_input(&current.borrow_and_update());
    sender.send(input).await.is_ok() && current.changed().await.is_ok()
}

fn host_desk(
    options: &AppOptions,
    registry: &Arc<HostRegistry>,
    tokens: Arc<dyn TokenStore>,
) -> Result<shell_data::HostDesk> {
    let notes = shell_data::HostNotes::open(options.state_dir().join("host-notes.json"))
        .map_err(|e| Error::Daemon(e.to_string()))?;
    Ok(shell_data::HostDesk {
        book: Arc::new(TransportBook::new(Arc::clone(registry), tokens)),
        notes: Arc::new(notes),
    })
}

async fn build_transport(
    options: &AppOptions,
    locator: watch::Receiver<Option<PathBuf>>,
) -> Result<(
    Arc<dyn DaemonTransport>,
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
    Ok((Arc::new(shell), registry, tokens))
}

#[cfg(test)]
mod tests {
    use std::path::Path;

    use super::*;

    fn options(home: &Path) -> AppOptions {
        AppOptions {
            panes_dir: None,
            chat_entry: "chat/index.html".into(),
            home: home.to_path_buf(),
            daemon_binary: Some(home.join("missing-daemon")),
        }
    }

    fn retry_after_a_halt(link: &DaemonLink) {
        link.phases().recv_blocking().unwrap();
        link.restart();
    }

    const CASES: &[(&str, usize)] = &[
        ("a missing daemon binary is reported as a halt", 0),
        ("Retry after a failed launch makes a fresh attempt", 1),
        ("every Retry reports its own failed attempt", 2),
    ];

    #[test]
    fn every_failed_launch_is_reported_as_a_halt() {
        for (name, retries) in CASES {
            let home = tempfile::tempdir().unwrap();
            let link = DaemonLink::start(&options(home.path())).unwrap();
            (0..*retries).for_each(|_| retry_after_a_halt(&link));
            let phase = link.phases().recv_blocking().unwrap();
            assert!(
                matches!(phase, StartupInput::Halted { .. }),
                "{name}: the startup view explains it: {phase:?}"
            );
            assert!(link.log().is_none(), "{name}: nothing to open");
        }
    }

    #[test]
    fn shutdown_without_a_daemon_is_a_no_op_and_repeatable() {
        let home = tempfile::tempdir().unwrap();
        let link = DaemonLink::start(&options(home.path())).unwrap();
        link.shutdown();
        link.shutdown();
        assert!(link.supervisor().is_none(), "nothing left to stop");
    }
}
