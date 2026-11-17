use std::path::PathBuf;
use std::sync::Arc;

use daemon_supervisor::{DaemonEvent, DaemonPaths, Supervisor, SupervisorConfig, locate_binary};
use startup_view::StartupInput;
use tokio::sync::watch;

use crate::app_options::AppOptions;
use crate::error::{Error, Result};
use crate::link_state::LinkState;
use crate::phase_input::phase_input;

pub(super) async fn try_launch(
    options: &AppOptions,
    state: &LinkState,
    sender: &async_channel::Sender<StartupInput>,
) {
    match launch(options, sender.clone()).await {
        Ok((supervisor, paths)) => state.adopt(supervisor, paths),
        Err(error) => report_halt(sender, &error),
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

fn current_binary() -> Option<PathBuf> {
    match std::env::current_exe() {
        Ok(path) => Some(path),
        Err(error) => {
            tracing::warn!(%error, "the running binary's path is unknown");
            None
        },
    }
}

async fn launch(
    options: &AppOptions,
    sender: async_channel::Sender<StartupInput>,
) -> Result<(Arc<Supervisor>, DaemonPaths)> {
    let current = current_binary();
    let path_var = std::env::var_os("PATH").filter(|_| cfg!(debug_assertions));
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::link_state::linked;
    use crate::test_support::options_without_a_daemon;

    #[test]
    fn a_missing_daemon_binary_is_reported_as_a_halt_and_adopts_nothing() {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        let (state, _locator) = linked();
        let (sender, phases) = async_channel::unbounded();
        runtime.block_on(try_launch(&options_without_a_daemon(), &state, &sender));
        let phase = phases.try_recv().unwrap();
        assert!(matches!(phase, StartupInput::Halted { .. }), "{phase:?}");
        assert!(state.supervisor().is_none(), "nothing launched");
        assert!(state.log().is_none(), "nothing to open");
    }
}
