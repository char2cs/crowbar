use std::sync::Arc;

use super::DaemonLink;
use super::launch::try_launch;
use crate::app_options::AppOptions;
use crate::link_state::LinkState;

impl DaemonLink {
    pub(crate) fn shutdown(&self) {
        let Some(supervisor) = self.state.take_supervisor() else {
            return;
        };
        match Arc::try_unwrap(supervisor) {
            Ok(owned) => self.runtime.block_on(owned.stop()),
            Err(shared) => {
                tracing::warn!(
                    holders = Arc::strong_count(&shared),
                    "a restart is still running; the daemon will be killed on exit"
                );
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
    sender: &async_channel::Sender<startup::Input>,
) {
    let _turn = state.turn().await;
    if state.supervisor().is_none() {
        try_launch(options, state, sender).await;
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::link_state::linked;
    use crate::test_support::options_without_a_daemon;

    const ATTEMPTS: &[(&str, usize)] = &[
        ("Retry after a failed launch makes a fresh attempt", 1),
        ("every Retry reports its own failed attempt", 2),
    ];

    fn retry(
        runtime: &tokio::runtime::Runtime,
        state: &LinkState,
        sender: &async_channel::Sender<startup::Input>,
    ) {
        let options = options_without_a_daemon();
        runtime.block_on(relaunch_unless_running(&options, state, sender));
    }

    #[test]
    fn every_retry_without_a_daemon_reports_its_own_halt() {
        let runtime = tokio::runtime::Builder::new_current_thread()
            .enable_all()
            .build()
            .unwrap();
        for (name, attempts) in ATTEMPTS {
            let (state, _locator) = linked();
            let (sender, phases) = async_channel::unbounded();
            (0..*attempts).for_each(|_| retry(&runtime, &state, &sender));
            assert_eq!(phases.len(), *attempts, "{name}");
        }
    }
}
