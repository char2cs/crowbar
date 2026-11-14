use protocol::PaneOrigin;
use tokio::time::Instant;

use super::*;
use crate::endpoint::Endpoint;
use crate::exact_origins::ExactOrigins;
use crate::host_registry::HostRegistry;
use crate::memory_token_store::MemoryTokenStore;

fn state_with(registry: HostRegistry) -> RelayState {
    let transport = Transport::new(
        Arc::new(std::env::temp_dir().join("cb-relay-state.sock")),
        Arc::new(registry),
        Arc::new(MemoryTokenStore::default()),
    );
    RelayState {
        transport: Arc::new(transport),
        config: RelayConfig::default(),
        policy: Arc::new(ExactOrigins::new(vec![], "crowbar-ext://{extension}")),
        tokens: Mutex::new(TokenBook::new(std::time::Duration::from_secs(60))),
        connections: ConnectionBook::new(4, 4),
        port: 1,
        shutdown: CancellationToken::new(),
    }
}

fn scope(pane: &str, host: &str) -> RelayScope {
    RelayScope::new(
        HostId::new(host).unwrap(),
        PaneId::new(pane).unwrap(),
        PaneOrigin::Core,
    )
}

fn issue(state: &RelayState, scope: RelayScope) -> String {
    state.tokens().issue(scope, Instant::now()).unwrap()
}

#[tokio::test]
async fn a_pane_revoked_between_the_token_check_and_the_open_stream_leaves_no_stream() {
    let state = state_with(HostRegistry::in_memory());
    let token = issue(&state, scope("p1", "local"));
    let (_, guard) = state.admit(&token, |_| Ok(())).unwrap();
    state.revoke_pane(&PaneId::new("p1").unwrap());
    assert!(
        guard.cancelled().is_cancelled(),
        "registered before the revoke, so cancelled by it"
    );
    assert_eq!(
        state.admit(&token, |_| Ok(())).err(),
        Some(StatusCode::UNAUTHORIZED),
        "revoked before the check, so refused"
    );
}

#[tokio::test]
async fn a_refusing_policy_registers_nothing() {
    let state = state_with(HostRegistry::in_memory());
    let token = issue(&state, scope("p1", "local"));
    assert_eq!(
        state.admit(&token, |_| Err(StatusCode::FORBIDDEN)).err(),
        Some(StatusCode::FORBIDDEN)
    );
    assert!(
        state
            .connections
            .register(&scope("p1", "local"), &state.shutdown)
            .is_some(),
        "no slot was taken by the refused dial"
    );
}

#[tokio::test]
async fn removing_a_host_revokes_its_tokens_and_streams_but_not_local_ones() {
    let state = state_with(HostRegistry::in_memory());
    let remote = issue(&state, scope("p1", "host-1"));
    let local = issue(&state, scope("p2", "local"));
    let (_, remote_guard) = state.admit(&remote, |_| Ok(())).unwrap();
    let (_, local_guard) = state.admit(&local, |_| Ok(())).unwrap();
    state.revoke_host(&HostId::new("host-1").unwrap());
    assert!(remote_guard.cancelled().is_cancelled());
    assert!(!local_guard.cancelled().is_cancelled());
}

#[tokio::test]
async fn reconciling_drops_every_host_the_registry_no_longer_lists() {
    let registry = HostRegistry::in_memory();
    registry
        .insert_blocking(
            HostId::new("kept").unwrap(),
            Endpoint::parse("10.0.0.2:1").unwrap(),
        )
        .unwrap();
    let state = state_with(registry);
    let kept = issue(&state, scope("p1", "kept"));
    let gone = issue(&state, scope("p2", "gone"));
    let local = issue(&state, scope("p3", "local"));
    let (_, gone_guard) = state.admit(&gone, |_| Ok(())).unwrap();
    state.revoke_unregistered_hosts();
    assert!(gone_guard.cancelled().is_cancelled());
    assert_eq!(
        state.admit(&gone, |_| Ok(())).err(),
        Some(StatusCode::UNAUTHORIZED)
    );
    assert!(state.admit(&kept, |_| Ok(())).is_ok());
    assert!(state.admit(&local, |_| Ok(())).is_ok());
}
