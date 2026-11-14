use protocol::{ExtensionId, PaneKind};

use super::*;

fn scope(pane: &str, host: &str) -> RelayScope {
    RelayScope {
        host: HostId::new(host).unwrap(),
        pane: PaneId::new(pane).unwrap(),
        kind: PaneKind::Core,
        extension: None::<ExtensionId>,
    }
}

#[tokio::test(start_paused = true)]
async fn a_token_names_its_scope_until_it_expires() {
    let mut book = TokenBook::new(Duration::from_secs(60));
    let token = book.issue(scope("p1", "local"), Instant::now()).unwrap();
    assert_eq!(
        book.authorize(&token, Instant::now()),
        Some(scope("p1", "local"))
    );
    tokio::time::advance(Duration::from_secs(61)).await;
    assert_eq!(book.authorize(&token, Instant::now()), None);
}

#[tokio::test(start_paused = true)]
async fn tokens_are_long_random_and_distinct() {
    let mut book = TokenBook::new(Duration::from_secs(60));
    let one = book.issue(scope("p1", "local"), Instant::now()).unwrap();
    let two = book.issue(scope("p2", "local"), Instant::now()).unwrap();
    assert_ne!(one, two);
    assert_eq!(one.len(), 43);
    assert!(
        one.bytes()
            .all(|b| b.is_ascii_alphanumeric() || b == b'-' || b == b'_')
    );
    assert_eq!(book.authorize("not-a-token", Instant::now()), None);
}

#[tokio::test(start_paused = true)]
async fn issuing_again_for_the_same_stream_replaces_the_old_token() {
    let mut book = TokenBook::new(Duration::from_secs(60));
    let old = book.issue(scope("p1", "local"), Instant::now()).unwrap();
    let other_host = book.issue(scope("p1", "host-1"), Instant::now()).unwrap();
    let fresh = book.issue(scope("p1", "local"), Instant::now()).unwrap();
    assert_eq!(book.authorize(&old, Instant::now()), None);
    assert!(book.authorize(&fresh, Instant::now()).is_some());
    assert!(book.authorize(&other_host, Instant::now()).is_some());
}

#[tokio::test(start_paused = true)]
async fn revoking_a_pane_or_a_host_drops_exactly_their_tokens() {
    let mut book = TokenBook::new(Duration::from_secs(60));
    let a = book.issue(scope("p1", "local"), Instant::now()).unwrap();
    let b = book.issue(scope("p2", "host-1"), Instant::now()).unwrap();
    let c = book.issue(scope("p3", "local"), Instant::now()).unwrap();
    book.revoke_pane(&PaneId::new("p1").unwrap());
    book.revoke_host(&HostId::new("host-1").unwrap());
    assert_eq!(book.authorize(&a, Instant::now()), None);
    assert_eq!(book.authorize(&b, Instant::now()), None);
    assert!(book.authorize(&c, Instant::now()).is_some());
}
