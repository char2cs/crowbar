mod admission;
mod bridge;
mod gate;

use std::sync::Arc;

use futures_util::SinkExt;
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::sync::{OwnedSemaphorePermit, oneshot};
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;
use tokio_tungstenite::{WebSocketStream, accept_hdr_async_with_config};
use tokio_util::sync::CancellationToken;

use crate::error::Error;
use crate::hosts::Target;
use crate::relay::session::admission::Admission;
use crate::relay::session::bridge::bridge;
use crate::relay::session::bridge::close_message::close_message;
use crate::relay::session::bridge::limits::Limits;
use crate::relay::session::gate::Gate;
use crate::relay::state::State;
use crate::transport::UpstreamRequest;
use crate::transport::{UpstreamSocket, ws_config};

pub(crate) async fn run<S>(
    state: Arc<State>,
    stream: S,
    handshake_slot: OwnedSemaphorePermit,
) where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    let (admitted, admission) = oneshot::channel();
    let gate = Gate::new(Arc::clone(&state), admitted);
    let config = ws_config(state.config.max_message_bytes);
    let handshake = accept_hdr_async_with_config(stream, gate, Some(config));
    let client = match tokio::time::timeout(state.config.handshake_timeout, handshake).await {
        Ok(Ok(client)) => client,
        Ok(Err(e)) => return tracing::debug!("relay handshake refused: {e}"),
        Err(elapsed) => return tracing::debug!("relay handshake timed out: {elapsed}"),
    };
    drop(handshake_slot);
    let Ok(admission) = admission.await else {
        return tracing::debug!("relay handshake finished without an admission");
    };
    serve(&state, client, admission).await;
}

async fn open_unless_revoked(
    state: &State,
    wanted: &UpstreamRequest,
    cancel: &CancellationToken,
) -> Option<Result<UpstreamSocket, Error>> {
    let open = state.transport.open_stream(wanted);
    let limited = tokio::time::timeout(state.config.open_timeout, open);
    let opened = cancel.run_until_cancelled(limited).await?;
    Some(opened.unwrap_or(Err(Error::UpgradeTimeout)))
}

async fn refuse_upstream<S>(
    state: &State,
    client: WebSocketStream<S>,
    error: &Error,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    tracing::debug!("relay upstream refused: {error}");
    let reason = error.marker().unwrap_or("daemon-unavailable");
    say_close(client, CloseCode::Error, reason, state).await;
}

async fn serve<S>(
    state: &State,
    client: WebSocketStream<S>,
    admission: Admission,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let Admission {
        scope,
        path_and_query,
        upstream_idle,
        guard,
    } = admission;
    let cancel = guard.cancelled();
    let wanted = UpstreamRequest {
        target: Target::of(scope.host()),
        origin: scope.origin().clone(),
        path_and_query,
        max_message: state.config.max_message_bytes,
    };
    match open_unless_revoked(state, &wanted, &cancel).await {
        None => say_close(client, CloseCode::Policy, "revoked", state).await,
        Some(Ok(upstream)) => {
            let limits = Limits::of(&state.config, upstream_idle);
            bridge(client, upstream, cancel, limits).await;
        },
        Some(Err(e)) => refuse_upstream(state, client, &e).await,
    }
}

async fn farewell<S>(
    client: &mut WebSocketStream<S>,
    code: CloseCode,
    reason: &'static str,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    if client.send(close_message(code, reason)).await.is_err() {
        return;
    }
    if let Err(e) = client.close(None).await {
        tracing::debug!("close not delivered: {e}");
    }
}

async fn say_close<S>(
    mut client: WebSocketStream<S>,
    code: CloseCode,
    reason: &'static str,
    state: &State,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let say = farewell(&mut client, code, reason);
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

    use tokio::io::{AsyncWriteExt, duplex};
    use tokio::sync::Semaphore;

    use super::*;
    use crate::hosts::HostRegistry;
    use crate::relay::connections::Book;
    use crate::relay::grants::ExactOrigins;
    use crate::relay::grants::TokenBook;
    use crate::relay::relay_config::RelayConfig;
    use crate::relay::state::adapters::tokio_clock::tokio_now;
    use crate::transport::MemoryTokenStore;
    use crate::transport::Transport;

    fn state(handshake_timeout: Duration) -> Arc<State> {
        let transport = Transport::new(
            Arc::new(std::path::PathBuf::from("/nonexistent/cb-session.sock")),
            Arc::new(HostRegistry::in_memory()),
            Arc::new(MemoryTokenStore::default()),
        );
        let config = RelayConfig {
            handshake_timeout,
            ..RelayConfig::default()
        };
        Arc::new(State {
            transport: Arc::new(transport),
            config,
            policy: Arc::new(ExactOrigins::new(vec![], "crowbar-ext://{extension}")),
            tokens: Mutex::new(TokenBook::new(Duration::from_secs(60))),
            connections: Book::new(4, 4),
            port: 1,
            shutdown: CancellationToken::new(),
            clock: Arc::new(tokio_now),
        })
    }

    fn slot() -> OwnedSemaphorePermit {
        Arc::new(Semaphore::new(1)).try_acquire_owned().unwrap()
    }

    const PIPE_BYTES: usize = 4096;

    #[tokio::test(start_paused = true)]
    async fn a_client_that_never_finishes_the_handshake_is_dropped_at_the_deadline() {
        let (silent, accepted) = duplex(PIPE_BYTES);
        run(state(Duration::from_secs(10)), accepted, slot()).await;
        drop(silent);
    }

    #[tokio::test]
    async fn garbage_instead_of_a_websocket_handshake_ends_the_session() {
        let (mut client, accepted) = duplex(PIPE_BYTES);
        client.write_all(b"GET / HTTP/1.1\r\n\r\n").await.unwrap();
        run(state(Duration::from_secs(10)), accepted, slot()).await;
    }
}
