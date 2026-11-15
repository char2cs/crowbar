use std::net::SocketAddr;
use std::sync::{Arc, Mutex};
use std::time::Duration;

use protocol::{HostId, PaneId, RelayEndpoint, RelayScope};
use tokio::net::TcpListener;
use tokio::sync::Semaphore;
use tokio::sync::broadcast::{self, error::RecvError};
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use crate::connection_book::ConnectionBook;
use crate::error::Error;
use crate::origin_policy::OriginPolicy;
use crate::relay_config::RelayConfig;
use crate::relay_state::RelayState;
use crate::session;
use crate::stream_access::StreamAccess;
use crate::token_book::TokenBook;
use crate::transport::Transport;

const ACCEPT_BACKOFF: Duration = Duration::from_millis(50);

/// The loopback WebSocket relay: listens on `127.0.0.1` on a random port, checks each pane's
/// token, `Host` and `Origin` before dialling the daemon, then bridges frames unchanged so
/// binary stays binary. Dropping it closes every stream.
pub struct Relay {
    state: Arc<RelayState>,
}

impl Relay {
    /// Binds the relay and starts serving.
    ///
    /// # Errors
    /// [`Error::Bind`] when no loopback port can be bound.
    pub async fn start(
        transport: Arc<Transport>,
        policy: Arc<dyn OriginPolicy>,
        config: RelayConfig,
    ) -> Result<Self, Error> {
        let removals = transport.removed_hosts();
        let listener = TcpListener::bind(("127.0.0.1", 0))
            .await
            .map_err(Error::Bind)?;
        let port = listener.local_addr().map_err(Error::Bind)?.port();
        let state = Arc::new(RelayState {
            connections: ConnectionBook::new(config.max_connections, config.max_per_pane),
            tokens: Mutex::new(TokenBook::new(config.token_ttl)),
            shutdown: CancellationToken::new(),
            transport,
            config,
            policy,
            port,
        });
        tokio::spawn(accept_loop(listener, Arc::clone(&state)));
        tokio::spawn(follow_removals(Arc::clone(&state), removals));
        Ok(Self { state })
    }

    /// The address the relay listens on.
    #[must_use]
    pub fn addr(&self) -> SocketAddr {
        SocketAddr::from(([127, 0, 0, 1], self.state.port))
    }

    /// Stops accepting and closes every open stream.
    pub fn shutdown(&self) {
        self.state.shutdown.cancel();
    }
}

impl std::fmt::Debug for Relay {
    fn fmt(
        &self,
        f: &mut std::fmt::Formatter<'_>,
    ) -> std::fmt::Result {
        f.debug_struct("Relay")
            .field("addr", &self.addr())
            .finish_non_exhaustive()
    }
}

impl Drop for Relay {
    fn drop(&mut self) {
        self.state.shutdown.cancel();
    }
}

impl StreamAccess for Relay {
    fn issue(
        &self,
        scope: &RelayScope,
    ) -> Result<RelayEndpoint, Error> {
        let token = self.state.tokens().issue(scope.clone(), Instant::now())?;
        Ok(RelayEndpoint::new(
            format!("ws://127.0.0.1:{}", self.state.port),
            token,
        ))
    }

    fn revoke_pane(
        &self,
        pane: &PaneId,
    ) {
        self.state.revoke_pane(pane);
    }
}

async fn accept_loop(
    listener: TcpListener,
    state: Arc<RelayState>,
) {
    let handshakes = Arc::new(Semaphore::new(state.config.max_handshakes));
    loop {
        let accepted = tokio::select! {
            () = state.shutdown.cancelled() => return,
            accepted = listener.accept() => accepted,
        };
        let Ok((tcp, _peer)) = accepted else {
            tokio::time::sleep(ACCEPT_BACKOFF).await;
            continue;
        };
        let Ok(permit) = Arc::clone(&handshakes).try_acquire_owned() else {
            turn_away(&tcp);
            continue;
        };
        tokio::spawn(session::run(Arc::clone(&state), tcp, permit));
    }
}

fn turn_away(tcp: &tokio::net::TcpStream) {
    const BUSY: &[u8] =
        b"HTTP/1.1 503 Service Unavailable\r\nConnection: close\r\nContent-Length: 0\r\n\r\n";
    if let Err(e) = tcp.try_write(BUSY) {
        tracing::debug!("relay is busy and the refusal was not delivered: {e}");
    }
}

async fn follow_removals(
    state: Arc<RelayState>,
    mut removed: broadcast::Receiver<HostId>,
) {
    loop {
        let host = tokio::select! {
            () = state.shutdown.cancelled() => return,
            host = removed.recv() => host,
        };
        match host {
            Ok(host) => state.revoke_host(&host),
            Err(RecvError::Lagged(_)) => state.revoke_unregistered_hosts(),
            Err(RecvError::Closed) => return,
        }
    }
}

#[cfg(test)]
mod tests {
    use protocol::{HostId, PaneId, PaneOrigin};
    use tokio::sync::broadcast;

    use super::*;
    use crate::exact_origins::ExactOrigins;
    use crate::host_registry::HostRegistry;
    use crate::memory_token_store::MemoryTokenStore;

    fn transport() -> Arc<Transport> {
        Arc::new(Transport::new(
            Arc::new(std::env::temp_dir().join("cb-relay-unit.sock")),
            Arc::new(HostRegistry::in_memory()),
            Arc::new(MemoryTokenStore::default()),
        ))
    }

    async fn relay_over(transport: Arc<Transport>) -> Relay {
        let policy = Arc::new(ExactOrigins::new(vec![], "crowbar-ext://{extension}"));
        Relay::start(transport, policy, RelayConfig::default())
            .await
            .unwrap()
    }

    async fn relay() -> Relay {
        relay_over(transport()).await
    }

    fn scope_on(
        host: &str,
        origin: PaneOrigin,
    ) -> RelayScope {
        RelayScope::new(
            HostId::new(host).unwrap(),
            PaneId::new("p1").unwrap(),
            origin,
        )
    }

    fn scope(origin: PaneOrigin) -> RelayScope {
        scope_on("local", origin)
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
        let endpoint = relay.issue(&scope(PaneOrigin::Core)).unwrap();
        assert_eq!(
            endpoint.url(),
            format!("ws://127.0.0.1:{}", relay.addr().port())
        );
        assert_eq!(endpoint.token().len(), 43);
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
        let remote = scope_on("host-1", PaneOrigin::Core);
        let other = scope(PaneOrigin::Core);
        let connections = &relay.state.connections;
        let doomed_guard = connections
            .register(&remote, &relay.state.shutdown)
            .unwrap();
        let kept_guard = connections.register(&other, &relay.state.shutdown).unwrap();
        let (doomed, kept) = (doomed_guard.cancelled(), kept_guard.cancelled());
        transport.remove_host(remote.host()).await.unwrap();
        tokio::time::timeout(Duration::from_secs(30), doomed.cancelled())
            .await
            .unwrap();
        assert!(!kept.is_cancelled());
    }

    #[tokio::test]
    async fn a_follower_that_fell_behind_revokes_every_host_the_registry_no_longer_lists() {
        let relay = relay().await;
        let state = Arc::clone(&relay.state);
        let gone = scope_on("host-9", PaneOrigin::Core);
        let guard = state.connections.register(&gone, &state.shutdown).unwrap();
        let token = relay.issue(&gone).unwrap();
        let (sender, receiver) = broadcast::channel(2);
        for n in 0..6 {
            sender
                .send(HostId::new(format!("other-{n}")).unwrap())
                .unwrap();
        }
        let follower = tokio::spawn(follow_removals(Arc::clone(&state), receiver));
        tokio::time::timeout(Duration::from_secs(30), guard.cancelled().cancelled())
            .await
            .unwrap();
        assert!(
            state.admit(token.token(), |_| Ok(())).is_err(),
            "its token is gone too"
        );
        state.shutdown.cancel();
        follower.await.unwrap();
    }

    #[tokio::test]
    async fn a_handshake_beyond_the_limit_is_answered_503_not_left_hanging() {
        use tokio::io::AsyncReadExt;
        let transport = transport();
        let policy = Arc::new(ExactOrigins::new(vec![], "crowbar-ext://{extension}"));
        let config = RelayConfig {
            max_handshakes: 1,
            handshake_timeout: Duration::from_secs(30),
            ..RelayConfig::default()
        };
        let relay = Relay::start(transport, policy, config).await.unwrap();
        let silent = tokio::net::TcpStream::connect(relay.addr()).await.unwrap();
        let mut refused = tokio::net::TcpStream::connect(relay.addr()).await.unwrap();
        let mut answer = Vec::new();
        tokio::time::timeout(Duration::from_secs(30), refused.read_to_end(&mut answer))
            .await
            .unwrap()
            .unwrap();
        assert!(answer.starts_with(b"HTTP/1.1 503"));
        drop(silent);
    }
}
