use std::io;
use std::net::{Ipv4Addr, SocketAddr};
use std::sync::{Arc, Mutex};
use std::time::Duration;

use protocol::{HostId, PaneId, RelayEndpoint, RelayScope};
use tokio::io::{AsyncRead, AsyncWrite, AsyncWriteExt};
use tokio::net::{TcpListener, TcpStream};
use tokio::sync::Semaphore;
use tokio::sync::broadcast::{self, error::RecvError};
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use crate::acceptor::Acceptor;
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
        let listener = TcpListener::bind((Ipv4Addr::LOCALHOST, 0))
            .await
            .map_err(Error::Bind)?;
        Self::start_on(listener, transport, policy, config)
    }

    /// Starts serving connections from `acceptor`, which panes reach at `127.0.0.1` on its
    /// [`Acceptor::local_port`]. [`Relay::start`] is this over a loopback TCP listener; tests
    /// pass an in-memory acceptor.
    ///
    /// # Errors
    /// [`Error::Bind`] when the acceptor does not know its port.
    pub fn start_on(
        acceptor: impl Acceptor,
        transport: Arc<Transport>,
        policy: Arc<dyn OriginPolicy>,
        config: RelayConfig,
    ) -> Result<Self, Error> {
        let port = acceptor.local_port().map_err(Error::Bind)?;
        let removals = transport.removed_hosts();
        let state = Arc::new(RelayState {
            connections: ConnectionBook::new(config.max_connections, config.max_per_pane),
            tokens: Mutex::new(TokenBook::new(config.token_ttl)),
            shutdown: CancellationToken::new(),
            transport,
            config,
            policy,
            port,
        });
        tokio::spawn(accept_loop(acceptor, Arc::clone(&state)));
        tokio::spawn(follow_removals(Arc::clone(&state), removals));
        Ok(Self { state })
    }

    /// The address the relay listens on.
    #[must_use]
    pub fn addr(&self) -> SocketAddr {
        SocketAddr::from((Ipv4Addr::LOCALHOST, self.state.port))
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

async fn accept_loop<A: Acceptor>(
    mut acceptor: A,
    state: Arc<RelayState>,
) {
    let handshakes = Arc::new(Semaphore::new(state.config.max_handshakes));
    while let Some(accepted) = state.shutdown.run_until_cancelled(acceptor.accept()).await {
        greet(&state, &handshakes, accepted).await;
    }
}

impl Acceptor for TcpListener {
    type Stream = TcpStream;

    fn local_port(&self) -> io::Result<u16> {
        Ok(self.local_addr()?.port())
    }

    async fn accept(&mut self) -> io::Result<TcpStream> {
        let (stream, _peer) = TcpListener::accept(self).await?;
        Ok(stream)
    }
}

async fn greet<S>(
    state: &Arc<RelayState>,
    handshakes: &Arc<Semaphore>,
    accepted: io::Result<S>,
) where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    let Ok(stream) = accepted else {
        return tokio::time::sleep(ACCEPT_BACKOFF).await;
    };
    let Ok(permit) = Arc::clone(handshakes).try_acquire_owned() else {
        return turn_away(stream, state.config.close_grace);
    };
    tokio::spawn(session::run(Arc::clone(state), stream, permit));
}

fn turn_away<S>(
    stream: S,
    grace: Duration,
) where
    S: AsyncWrite + Unpin + Send + 'static,
{
    tokio::spawn(refuse_busy(stream, grace));
}

async fn refuse_busy<S>(
    mut stream: S,
    grace: Duration,
) where
    S: AsyncWrite + Unpin,
{
    const BUSY: &[u8] =
        b"HTTP/1.1 503 Service Unavailable\r\nConnection: close\r\nContent-Length: 0\r\n\r\n";
    match tokio::time::timeout(grace, stream.write_all(BUSY)).await {
        Ok(Ok(())) => {},
        Ok(Err(e)) => tracing::debug!("relay is busy and the refusal was not delivered: {e}"),
        Err(_) => tracing::debug!("relay is busy and the refusal was not taken in time"),
    }
}

async fn follow_removals(
    state: Arc<RelayState>,
    mut removed: broadcast::Receiver<HostId>,
) {
    while let Some(host) = state.shutdown.run_until_cancelled(removed.recv()).await {
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

    const PORT: u16 = 4242;

    /// An acceptor nobody dials: the unit tests drive the relay's state directly.
    struct Idle {
        port: u16,
        incoming: tokio::sync::mpsc::Receiver<tokio::io::DuplexStream>,
        _dialer: tokio::sync::mpsc::Sender<tokio::io::DuplexStream>,
    }

    fn idle(port: u16) -> Idle {
        let (dialer, incoming) = tokio::sync::mpsc::channel(1);
        Idle {
            port,
            incoming,
            _dialer: dialer,
        }
    }

    impl Acceptor for Idle {
        type Stream = tokio::io::DuplexStream;

        fn local_port(&self) -> io::Result<u16> {
            Ok(self.port)
        }

        async fn accept(&mut self) -> io::Result<Self::Stream> {
            self.incoming
                .recv()
                .await
                .ok_or_else(|| io::Error::from(io::ErrorKind::BrokenPipe))
        }
    }

    fn relay_over(transport: Arc<Transport>) -> Relay {
        let policy = Arc::new(ExactOrigins::new(vec![], "crowbar-ext://{extension}"));
        Relay::start_on(idle(PORT), transport, policy, RelayConfig::default()).unwrap()
    }

    fn relay() -> Relay {
        relay_over(transport())
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

    #[tokio::test]
    async fn removing_a_host_cancels_its_open_streams_and_leaves_others_alone() {
        let transport = transport();
        let relay = relay_over(Arc::clone(&transport));
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

    fn overflow(sender: &broadcast::Sender<HostId>) {
        for n in 0..6 {
            sender
                .send(HostId::new(format!("other-{n}")).unwrap())
                .unwrap();
        }
    }

    #[tokio::test]
    async fn a_follower_that_fell_behind_revokes_every_host_the_registry_no_longer_lists() {
        let relay = relay();
        let state = Arc::clone(&relay.state);
        let gone = scope_on("host-9", PaneOrigin::Core);
        let guard = state.connections.register(&gone, &state.shutdown).unwrap();
        let token = relay.issue(&gone).unwrap();
        let (sender, receiver) = broadcast::channel(2);
        overflow(&sender);
        let follower = tokio::spawn(follow_removals(Arc::clone(&state), receiver));
        tokio::time::timeout(Duration::from_secs(30), guard.cancelled().cancelled())
            .await
            .unwrap();
        assert!(
            state.admit_stream(token.token(), |_| Ok(())).is_err(),
            "its token is gone too"
        );
        state.shutdown.cancel();
        follower.await.unwrap();
    }
}
