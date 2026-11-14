use protocol::{HostId, PaneId, PaneKind};

use super::*;
use crate::exact_origins::ExactOrigins;
use crate::host_registry::HostRegistry;
use crate::memory_token_store::MemoryTokenStore;

async fn relay() -> Relay {
    let transport = Transport::new(
        Arc::new(std::env::temp_dir().join("cb-relay-unit.sock")),
        Arc::new(HostRegistry::in_memory()),
        Arc::new(MemoryTokenStore::default()),
    );
    let policy = Arc::new(ExactOrigins::new(vec![], "crowbar-ext://{extension}"));
    Relay::start(Arc::new(transport), policy, RelayConfig::default())
        .await
        .unwrap()
}

fn scope(kind: PaneKind, extension: Option<&str>) -> RelayScope {
    RelayScope {
        host: HostId::new("local").unwrap(),
        pane: PaneId::new("p1").unwrap(),
        kind,
        extension: extension.map(|id| protocol::ExtensionId::new(id).unwrap()),
    }
}

async fn until_refused(addr: SocketAddr) {
    while tokio::net::TcpStream::connect(addr).await.is_ok() {
        tokio::task::yield_now().await;
    }
}

#[tokio::test]
async fn the_relay_listens_on_loopback_only_and_names_its_port_in_the_endpoint() {
    let relay = relay().await;
    assert!(relay.addr().ip().is_loopback());
    let endpoint = relay.issue(&scope(PaneKind::Core, None)).unwrap();
    assert_eq!(
        endpoint.url,
        format!("ws://127.0.0.1:{}", relay.addr().port())
    );
    assert_eq!(endpoint.token.len(), 43);
}

#[tokio::test]
async fn a_scope_that_contradicts_itself_gets_no_token() {
    let relay = relay().await;
    assert!(matches!(
        relay.issue(&scope(PaneKind::Core, Some("acme"))),
        Err(Error::BadGrant(_))
    ));
    assert!(matches!(
        relay.issue(&scope(PaneKind::Extension, None)),
        Err(Error::BadGrant(_))
    ));
}

#[tokio::test]
async fn shutting_down_stops_accepting_connections() {
    let relay = relay().await;
    let addr = relay.addr();
    relay.shutdown();
    let refused = tokio::time::timeout(Duration::from_secs(30), until_refused(addr)).await;
    assert!(refused.is_ok(), "the listener stayed open after shutdown");
}

#[tokio::test]
async fn removing_a_host_cancels_its_open_streams_and_leaves_others_alone() {
    let transport = transport();
    let relay = relay_over(Arc::clone(&transport)).await;
    let mut remote = scope(PaneKind::Core, None);
    remote.host = HostId::new("host-1").unwrap();
    let other = scope(PaneKind::Core, None);
    let connections = &relay.state.connections;
    let doomed_guard = connections.register(&remote, &relay.state.shutdown).unwrap();
    let kept_guard = connections.register(&other, &relay.state.shutdown).unwrap();
    let (doomed, kept) = (doomed_guard.cancelled(), kept_guard.cancelled());
    transport.remove_host(&remote.host).unwrap();
    tokio::time::timeout(Duration::from_secs(30), doomed.cancelled()).await.unwrap();
    assert!(!kept.is_cancelled());
}
