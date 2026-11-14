use protocol::{HostId, PaneId, PaneKind, RelayScope};

use super::*;

fn scope() -> RelayScope {
    RelayScope {
        host: HostId::new("local").unwrap(),
        pane: PaneId::new("p1").unwrap(),
        kind: PaneKind::Core,
        extension: None,
    }
}

#[test]
fn dropping_the_guard_frees_its_slot() {
    let book = ConnectionBook::new(1, 1);
    let root = CancellationToken::new();
    let guard = book.register(&scope(), &root).unwrap();
    assert!(book.register(&scope(), &root).is_none());
    drop(guard);
    assert!(book.register(&scope(), &root).is_some());
}

#[test]
fn the_guard_hands_out_the_cancellation_of_its_own_stream() {
    let book = ConnectionBook::new(2, 2);
    let root = CancellationToken::new();
    let guard = book.register(&scope(), &root).unwrap();
    let cancelled = guard.cancelled();
    assert!(!cancelled.is_cancelled());
    book.close_pane(&PaneId::new("p1").unwrap());
    assert!(cancelled.is_cancelled());
}
