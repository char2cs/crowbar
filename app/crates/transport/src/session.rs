use std::sync::Arc;

use futures_util::SinkExt;
use tokio::net::TcpStream;
use tokio::sync::{OwnedSemaphorePermit, oneshot};
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;
use tokio_tungstenite::{WebSocketStream, accept_hdr_async_with_config};

use crate::admission::Admission;
use crate::bridge::{bridge, close_message};
use crate::bridge_limits::BridgeLimits;
use crate::error::Error;
use crate::gate::Gate;
use crate::host_target::HostTarget;
use crate::relay_state::RelayState;
use crate::upstream::ws_config;
use crate::upstream_request::UpstreamRequest;

pub(crate) async fn run(
    state: Arc<RelayState>,
    tcp: TcpStream,
    handshake_slot: OwnedSemaphorePermit,
) {
    let (admitted, admission) = oneshot::channel();
    let gate = Gate::new(Arc::clone(&state), admitted);
    let config = ws_config(state.config.max_message_bytes);
    let handshake = accept_hdr_async_with_config(tcp, gate, Some(config));
    let client = match tokio::time::timeout(state.config.handshake_timeout, handshake).await {
        Ok(Ok(client)) => client,
        Ok(Err(e)) => return tracing::debug!("relay handshake refused: {e}"),
        Err(_) => return tracing::debug!("relay handshake timed out"),
    };
    drop(handshake_slot);
    let Ok(admission) = admission.await else {
        return tracing::debug!("relay handshake finished without an admission");
    };
    serve(&state, client, admission).await;
}

async fn serve(
    state: &RelayState,
    client: WebSocketStream<TcpStream>,
    admission: Admission,
) {
    let Admission {
        scope,
        path_and_query,
        upstream_idle,
        guard,
    } = admission;
    let target = HostTarget::of(scope.host());
    let cancel = guard.cancelled();
    let wanted = UpstreamRequest {
        target,
        origin: scope.origin().clone(),
        path_and_query,
        max_message: state.config.max_message_bytes,
    };
    let open = state.transport.open_stream(&wanted);
    let opened = tokio::select! {
        () = cancel.cancelled() => None,
        opened = tokio::time::timeout(state.config.open_timeout, open) => {
            Some(opened.unwrap_or(Err(Error::UpgradeTimeout)))
        },
    };
    match opened {
        None => say_close(client, CloseCode::Policy, "revoked", state).await,
        Some(Ok(upstream)) => {
            let limits = BridgeLimits::of(&state.config, upstream_idle);
            bridge(client, upstream, cancel, limits).await;
        },
        Some(Err(e)) => {
            tracing::debug!("relay upstream refused: {e}");
            let reason = e.marker().unwrap_or("daemon-unavailable");
            say_close(client, CloseCode::Error, reason, state).await;
        },
    }
}

async fn say_close(
    mut client: WebSocketStream<TcpStream>,
    code: CloseCode,
    reason: &'static str,
    state: &RelayState,
) {
    let say = async {
        if client.send(close_message(code, reason)).await.is_ok()
            && let Err(e) = client.close(None).await
        {
            tracing::debug!("close not delivered: {e}");
        }
    };
    if tokio::time::timeout(state.config.close_grace, say)
        .await
        .is_err()
    {
        tracing::debug!("client did not take the close within the grace period");
    }
}

#[cfg(test)]
mod tests {
    use std::sync::Mutex;
    use std::time::Duration;

    use tokio::net::{TcpListener, TcpStream};
    use tokio::sync::Semaphore;
    use tokio_util::sync::CancellationToken;

    use super::*;
    use crate::connection_book::ConnectionBook;
    use crate::exact_origins::ExactOrigins;
    use crate::host_registry::HostRegistry;
    use crate::memory_token_store::MemoryTokenStore;
    use crate::relay_config::RelayConfig;
    use crate::token_book::TokenBook;
    use crate::transport::Transport;

    fn state(handshake_timeout: Duration) -> Arc<RelayState> {
        let transport = Transport::new(
            Arc::new(std::env::temp_dir().join("cb-session.sock")),
            Arc::new(HostRegistry::in_memory()),
            Arc::new(MemoryTokenStore::default()),
        );
        Arc::new(RelayState {
            transport: Arc::new(transport),
            config: RelayConfig {
                handshake_timeout,
                ..RelayConfig::default()
            },
            policy: Arc::new(ExactOrigins::new(vec![], "crowbar-ext://{extension}")),
            tokens: Mutex::new(TokenBook::new(Duration::from_secs(60))),
            connections: ConnectionBook::new(4, 4),
            port: 1,
            shutdown: CancellationToken::new(),
        })
    }

    fn slot() -> OwnedSemaphorePermit {
        Arc::new(Semaphore::new(1)).try_acquire_owned().unwrap()
    }

    #[tokio::test(start_paused = true)]
    async fn a_client_that_never_finishes_the_handshake_is_dropped_at_the_deadline() {
        let listener = TcpListener::bind(("127.0.0.1", 0)).await.unwrap();
        let silent = TcpStream::connect(listener.local_addr().unwrap())
            .await
            .unwrap();
        let (accepted, _) = listener.accept().await.unwrap();
        run(state(Duration::from_secs(10)), accepted, slot()).await;
        drop(silent);
    }

    #[tokio::test]
    async fn garbage_instead_of_a_websocket_handshake_ends_the_session() {
        use tokio::io::AsyncWriteExt;
        let listener = TcpListener::bind(("127.0.0.1", 0)).await.unwrap();
        let mut client = TcpStream::connect(listener.local_addr().unwrap())
            .await
            .unwrap();
        let (accepted, _) = listener.accept().await.unwrap();
        client.write_all(b"GET / HTTP/1.1\r\n\r\n").await.unwrap();
        run(state(Duration::from_secs(10)), accepted, slot()).await;
    }
}
