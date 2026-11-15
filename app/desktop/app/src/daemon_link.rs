use std::path::{Path, PathBuf};
use std::sync::Arc;

use daemon_supervisor::{DaemonEvent, DaemonPaths, Supervisor, SupervisorConfig, locate_binary};
use protocol::DaemonTransport;
use startup_view::StartupInput;
use tokio::runtime::Runtime;
use transport::{
    ExactOrigins, HostRegistry, MemoryTokenStore, Relay, RelayConfig, ShellTransport, Transport,
};

use crate::app_options::AppOptions;
use crate::error::{Error, Result};
use crate::phase_input::phase_input;

const CORE_ORIGIN: &str = "crowbar://local";

/// The running daemon and the way panes reach it: the supervisor that keeps it alive, the
/// transport that carries requests to it, and the phases the startup view follows.
pub(crate) struct DaemonLink {
    runtime: Runtime,
    supervisor: Option<Arc<Supervisor>>,
    transport: Arc<dyn DaemonTransport>,
    phases: async_channel::Receiver<StartupInput>,
    log: Option<PathBuf>,
}

impl std::fmt::Debug for DaemonLink {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.debug_struct("DaemonLink")
            .field("supervised", &self.supervisor.is_some())
            .finish_non_exhaustive()
    }
}

impl DaemonLink {
    /// Starts the daemon named by `options` and the transport to it. A missing daemon binary is
    /// not an error here: the phases report it as a halt so the startup view explains it.
    pub(crate) fn start(options: &AppOptions) -> Result<Self> {
        let runtime = tokio::runtime::Builder::new_multi_thread()
            .worker_threads(2)
            .enable_all()
            .build()
            .map_err(|e| Error::Daemon(e.to_string()))?;
        let (sender, phases) = async_channel::unbounded();
        let launch = runtime.block_on(launch(options, sender.clone()));
        let (supervisor, socket, log) = match launch {
            Ok((supervisor, paths)) => (Some(supervisor), paths.socket, Some(paths.log)),
            Err(error) => {
                report_halt(&sender, &error);
                (None, options.state_dir().join("no-daemon.sock"), None)
            },
        };
        let transport = runtime.block_on(build_transport(options, socket))?;
        Ok(Self {
            runtime,
            supervisor,
            transport,
            phases,
            log,
        })
    }

    pub(crate) fn transport(&self) -> Arc<dyn DaemonTransport> {
        Arc::clone(&self.transport)
    }

    pub(crate) fn phases(&self) -> async_channel::Receiver<StartupInput> {
        self.phases.clone()
    }

    pub(crate) fn log(&self) -> Option<&Path> {
        self.log.as_deref()
    }

    /// Starts the daemon again after it halted or gave up.
    pub(crate) fn restart(&self) {
        let Some(supervisor) = self.supervisor.clone() else {
            return;
        };
        self.runtime.spawn(async move {
            supervisor
                .restart()
                .await
                .unwrap_or_else(|error| tracing::warn!(%error, "restarting the daemon failed"));
        });
    }
}

fn report_halt(sender: &async_channel::Sender<StartupInput>, error: &Error) {
    let halted = StartupInput::Halted {
        reason: None,
        message: error.to_string(),
    };
    if sender.try_send(halted).is_err() {
        tracing::warn!("the startup view is gone; the daemon failure goes unreported");
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

fn forward_phases(supervisor: &Supervisor, sender: async_channel::Sender<StartupInput>) {
    let mut current = supervisor.current();
    tokio::spawn(async move { while relay_phase(&sender, &mut current).await {} });
}

async fn relay_phase(
    sender: &async_channel::Sender<StartupInput>,
    current: &mut tokio::sync::watch::Receiver<DaemonEvent>,
) -> bool {
    let input = phase_input(&current.borrow_and_update());
    sender.send(input).await.is_ok() && current.changed().await.is_ok()
}

async fn build_transport(
    options: &AppOptions,
    socket: PathBuf,
) -> Result<Arc<dyn DaemonTransport>> {
    let registry = HostRegistry::open(options.state_dir().join("hosts.json"))
        .map_err(|e| Error::Daemon(e.to_string()))?;
    let transport = Arc::new(Transport::new(
        Arc::new(socket),
        Arc::new(registry),
        Arc::new(MemoryTokenStore::default()),
    ));
    let origins = ExactOrigins::new(vec![CORE_ORIGIN.to_owned()], CORE_ORIGIN);
    let relay = Relay::start(
        Arc::clone(&transport),
        Arc::new(origins),
        RelayConfig::default(),
    )
    .await
    .map_err(|e| Error::Daemon(e.to_string()))?;
    Ok(Arc::new(ShellTransport::new(
        tokio::runtime::Handle::current(),
        transport,
        Arc::new(relay),
    )))
}

#[cfg(test)]
#[path = "daemon_link_tests.rs"]
mod tests;
