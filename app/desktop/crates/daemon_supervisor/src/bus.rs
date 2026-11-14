use tokio::sync::{broadcast, watch};

use crate::daemon_event::DaemonEvent;

#[derive(Debug)]
pub(crate) struct Bus {
    stream: broadcast::Sender<DaemonEvent>,
    current: watch::Sender<DaemonEvent>,
}

impl Bus {
    pub(crate) fn new() -> Self {
        let (stream, _) = broadcast::channel(64);
        let (current, _) = watch::channel(DaemonEvent::Starting);
        Self { stream, current }
    }

    pub(crate) fn emit(&self, event: DaemonEvent) {
        tracing::info!("daemon: {event:?}");
        self.current.send_replace(event.clone());
        self.stream.send(event).ok();
    }

    pub(crate) fn subscribe(&self) -> broadcast::Receiver<DaemonEvent> {
        self.stream.subscribe()
    }

    pub(crate) fn current(&self) -> watch::Receiver<DaemonEvent> {
        self.current.subscribe()
    }
}

#[cfg(test)]
#[path = "bus_tests.rs"]
mod tests;
