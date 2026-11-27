use std::sync::{Arc, Mutex, PoisonError};

use tokio::sync::watch;

use crate::live::state::State;
use crate::live::state::change_signal::ChangeSignal;

pub(crate) struct Hub {
    state: Mutex<State>,
    signal: ChangeSignal,
    edits: watch::Sender<()>,
}

impl std::fmt::Debug for Hub {
    fn fmt(
        &self,
        formatter: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        formatter.debug_struct("Hub").finish_non_exhaustive()
    }
}

impl Hub {
    pub(crate) fn new(
        state: State,
        signal: ChangeSignal,
    ) -> Arc<Self> {
        Arc::new(Self {
            state: Mutex::new(state),
            signal,
            edits: watch::channel(()).0,
        })
    }

    pub(crate) fn read<R>(
        &self,
        read: impl FnOnce(&State) -> R,
    ) -> R {
        read(&self.state.lock().unwrap_or_else(PoisonError::into_inner))
    }

    pub(crate) fn edit<R>(
        &self,
        change: impl FnOnce(&mut State) -> R,
    ) -> R {
        let result = change(&mut self.state.lock().unwrap_or_else(PoisonError::into_inner));
        self.edits.send_replace(());
        (self.signal)();
        result
    }

    pub(crate) fn quietly<R>(
        &self,
        change: impl FnOnce(&mut State) -> R,
    ) -> R {
        change(&mut self.state.lock().unwrap_or_else(PoisonError::into_inner))
    }

    pub(crate) async fn until<R>(
        &self,
        mut ready: impl FnMut(&State) -> Option<R>,
    ) -> Option<R> {
        let mut found = None;
        let mut edits = self.edits.subscribe();
        let waited = edits
            .wait_for(|()| {
                found = self.read(&mut ready);
                found.is_some()
            })
            .await;
        if let Err(closed) = waited {
            tracing::debug!("the hub stopped before the awaited state arrived: {closed}");
        }
        found
    }
}

#[cfg(test)]
mod tests {
    use std::sync::atomic::{AtomicUsize, Ordering};

    use super::*;

    fn quiet_hub() -> Arc<Hub> {
        Hub::new(State::new(Vec::new()), Arc::new(|| {}))
    }

    async fn selected_after_waiting(hub: Arc<Hub>) -> Option<String> {
        hub.until(|state| state.selected_host.clone()).await
    }

    #[tokio::test]
    async fn until_waits_for_the_edit_that_makes_the_state_ready() {
        let hub = quiet_hub();
        let waiter = tokio::spawn(selected_after_waiting(Arc::clone(&hub)));
        hub.edit(|state| state.active_project = Some("p".into()));
        tokio::task::yield_now().await;
        assert!(!waiter.is_finished());
        hub.edit(|state| state.selected_host = Some("h".into()));
        assert_eq!(waiter.await.unwrap().as_deref(), Some("h"));
    }

    #[test]
    fn an_edit_signals_and_a_read_does_not() {
        let signals = Arc::new(AtomicUsize::new(0));
        let counter = Arc::clone(&signals);
        let hub = Hub::new(
            State::new(Vec::new()),
            Arc::new(move || {
                counter.fetch_add(1, Ordering::SeqCst);
            }),
        );
        assert!(hub.read(|state| state.hosts.is_empty()));
        assert_eq!(signals.load(Ordering::SeqCst), 0);
        hub.edit(|state| state.selected_host = Some("x".into()));
        assert_eq!(signals.load(Ordering::SeqCst), 1);
        assert_eq!(
            hub.read(|state| state.selected_host.clone()).as_deref(),
            Some("x")
        );
    }
}
