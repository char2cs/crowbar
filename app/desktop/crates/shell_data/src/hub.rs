//! The lock around the shared state, and the signal that follows every change.

use std::sync::{Arc, Mutex, PoisonError};

use crate::change_signal::ChangeSignal;
use crate::state::State;

pub(crate) struct Hub {
    state: Mutex<State>,
    signal: ChangeSignal,
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
        (self.signal)();
        result
    }
}

#[cfg(test)]
mod tests {
    use std::sync::atomic::{AtomicUsize, Ordering};

    use super::*;

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
