use std::net::SocketAddr;
use std::sync::{Arc, Mutex, PoisonError};
use std::time::Duration;

use protocol::{PaneId, RelayEndpoint, RelayScope};
use tokio::net::TcpListener;
use tokio::sync::Semaphore;
use tokio::sync::broadcast::error::RecvError;
use tokio::time::Instant;
use tokio_util::sync::CancellationToken;

use crate::access_scope::AccessScope;
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
        tokio::spawn(follow_removals(Arc::clone(&state)));
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
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
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
    fn issue(&self, scope: &RelayScope) -> Result<RelayEndpoint, Error> {
        AccessScope::of(scope.kind, scope.extension.as_ref())?;
        let token = self
            .state
            .tokens
            .lock()
            .unwrap_or_else(PoisonError::into_inner)
            .issue(scope.clone(), Instant::now())?;
        Ok(RelayEndpoint {
            url: format!("ws://127.0.0.1:{}", self.state.port),
            token,
        })
    }

    fn revoke_pane(&self, pane: &PaneId) {
        let mut tokens = self
            .state
            .tokens
            .lock()
            .unwrap_or_else(PoisonError::into_inner);
        tokens.revoke_pane(pane);
        drop(tokens);
        self.state.connections.close_pane(pane);
    }
}

async fn accept_loop(listener: TcpListener, state: Arc<RelayState>) {
    let handshakes = Arc::new(Semaphore::new(
        state.config.max_connections.saturating_mul(2),
    ));
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
            continue;
        };
        let state = Arc::clone(&state);
        tokio::spawn(async move {
            session::run(state, tcp).await;
            drop(permit);
        });
    }
}

async fn follow_removals(state: Arc<RelayState>) {
    let mut removed = state.transport.removed_hosts();
    loop {
        let host = tokio::select! {
            () = state.shutdown.cancelled() => return,
            host = removed.recv() => host,
        };
        match host {
            Ok(host) => {
                state
                    .tokens
                    .lock()
                    .unwrap_or_else(PoisonError::into_inner)
                    .revoke_host(&host);
                state.connections.close_host(&host);
            },
            Err(RecvError::Lagged(_)) => {},
            Err(RecvError::Closed) => return,
        }
    }
}

#[cfg(test)]
#[path = "relay_tests.rs"]
mod tests;
