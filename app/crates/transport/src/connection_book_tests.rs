use protocol::PaneKind;

use super::*;

fn scope(pane: &str, host: &str) -> RelayScope {
    RelayScope {
        host: HostId::new(host).unwrap(),
        pane: PaneId::new(pane).unwrap(),
        kind: PaneKind::Core,
        extension: None,
    }
}

#[test]
fn the_total_and_per_pane_limits_hold_and_free_up_on_drop() {
    let book = ConnectionBook::new(3, 2);
    let root = CancellationToken::new();
    let first = book.register(&scope("p1", "local"), &root).unwrap();
    let _second = book.register(&scope("p1", "local"), &root).unwrap();
    assert!(
        book.register(&scope("p1", "local"), &root).is_none(),
        "per pane"
    );
    let _third = book.register(&scope("p2", "local"), &root).unwrap();
    assert!(
        book.register(&scope("p3", "local"), &root).is_none(),
        "total"
    );
    drop(first);
    assert!(book.register(&scope("p3", "local"), &root).is_some());
}

#[test]
fn closing_a_pane_or_a_host_cancels_only_theirs() {
    let book = ConnectionBook::new(10, 10);
    let root = CancellationToken::new();
    let a = book.register(&scope("p1", "local"), &root).unwrap();
    let b = book.register(&scope("p2", "host-1"), &root).unwrap();
    let c = book.register(&scope("p3", "local"), &root).unwrap();
    book.close_pane(&PaneId::new("p1").unwrap());
    book.close_host(&HostId::new("host-1").unwrap());
    assert!(a.cancelled().is_cancelled());
    assert!(b.cancelled().is_cancelled());
    assert!(!c.cancelled().is_cancelled());
    assert_eq!(book.lock().live.len(), 3);
}

#[test]
fn shutting_the_root_down_cancels_every_connection() {
    let book = ConnectionBook::new(10, 10);
    let root = CancellationToken::new();
    let a = book.register(&scope("p1", "local"), &root).unwrap();
    root.cancel();
    assert!(a.cancelled().is_cancelled());
}
