use super::*;

#[test]
fn every_subscriber_sees_each_event_in_order_and_current_holds_the_latest() {
    let bus = Bus::new();
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
    Bus::new().emit(DaemonEvent::Unhealthy);
}
