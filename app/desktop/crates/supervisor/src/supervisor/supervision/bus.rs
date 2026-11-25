mod bus_event;

use tokio::sync::{broadcast, watch};

use crate::supervisor::daemon_event::DaemonEvent;
use crate::supervisor::supervision::bus::bus_event::BusEvent;

#[derive(Debug)]
pub(crate) struct Bus {
    stream: broadcast::Sender<DaemonEvent>,
    current: watch::Sender<DaemonEvent>,
}

const EVENT_BACKLOG: usize = 64;

impl Default for Bus {
    fn default() -> Self {
        let (stream, _) = broadcast::channel(EVENT_BACKLOG);
        let (current, _) = watch::channel(DaemonEvent::Starting);
        Self { stream, current }
    }
}

impl Bus {
    pub(crate) fn emit(
        &self,
        event: DaemonEvent,
    ) {
        crate::journal_sink::emit(&BusEvent::Emitted { event: &event });
        self.current.send_replace(event.clone());
        if self.stream.send(event).is_err() {
            crate::journal_sink::emit(&BusEvent::Unheard);
        }
    }

    pub(crate) fn subscribe(&self) -> broadcast::Receiver<DaemonEvent> {
        self.stream.subscribe()
    }

    pub(crate) fn current(&self) -> watch::Receiver<DaemonEvent> {
        self.current.subscribe()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn every_subscriber_sees_each_event_in_order_and_current_holds_the_latest() {
        let bus = Bus::default();
        let mut one = bus.subscribe();
        let mut two = bus.subscribe();
        let current = bus.current();
        assert_eq!(*current.borrow(), DaemonEvent::Starting);
        bus.emit(DaemonEvent::Ready { pid: Some(7) });
        bus.emit(DaemonEvent::Stopped);
        for receiver in [&mut one, &mut two] {
            assert_eq!(
                receiver.try_recv().unwrap(),
                DaemonEvent::Ready { pid: Some(7) }
            );
            assert_eq!(receiver.try_recv().unwrap(), DaemonEvent::Stopped);
        }
        assert_eq!(*current.borrow(), DaemonEvent::Stopped);
    }

    #[test]
    fn emitting_with_nobody_listening_is_harmless() {
        Bus::default().emit(DaemonEvent::Unhealthy);
    }
}
