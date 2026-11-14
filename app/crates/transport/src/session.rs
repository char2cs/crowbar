use std::sync::Arc;

use futures_util::SinkExt;
use tokio::net::TcpStream;
use tokio::sync::oneshot;
use tokio_tungstenite::tungstenite::protocol::frame::coding::CloseCode;
use tokio_tungstenite::{WebSocketStream, accept_hdr_async_with_config};

use crate::admission::Admission;
use crate::bridge::{bridge, close_message};
use crate::error::Error;
use crate::gate::Gate;
use crate::host_target::HostTarget;
use crate::relay_state::RelayState;
use crate::upstream::ws_config;

pub(crate) async fn run(state: Arc<RelayState>, tcp: TcpStream) {
    let (admitted, admission) = oneshot::channel();
    let gate = Gate::new(Arc::clone(&state), admitted);
    let config = ws_config(state.config.max_message_bytes);
    let handshake = accept_hdr_async_with_config(tcp, gate, Some(config));
    let client = match tokio::time::timeout(state.config.handshake_timeout, handshake).await {
        Ok(Ok(client)) => client,
        Ok(Err(e)) => return tracing::debug!("relay handshake refused: {e}"),
        Err(_) => return tracing::debug!("relay handshake timed out"),
    };
    let Ok(admission) = admission.await else {
        return tracing::debug!("relay handshake finished without an admission");
    };
    serve(&state, client, admission).await;
}

async fn serve(state: &RelayState, mut client: WebSocketStream<TcpStream>, admission: Admission) {
    let Some(guard) = state
        .connections
        .register(&admission.scope, &state.shutdown)
    else {
        client
            .send(close_message(CloseCode::Again, "too many streams"))
            .await
            .ok();
        return;
    };
    let target = HostTarget::of(&admission.scope.host);
    let opened = state
        .transport
        .open_stream(
            &target,
            &admission.path_and_query,
            state.config.max_message_bytes,
        )
        .await;
    match opened {
        Ok(upstream) => bridge(client, upstream, guard.cancelled(), &state.config).await,
        Err(e) => refuse(client, &e).await,
    }
}

async fn refuse(mut client: WebSocketStream<TcpStream>, error: &Error) {
    tracing::debug!("relay upstream refused: {error}");
    let reason = error.marker().unwrap_or("daemon-unavailable");
    client
        .send(close_message(CloseCode::Error, reason))
        .await
        .ok();
    client.close(None).await.ok();
}

#[cfg(test)]
#[path = "session_tests.rs"]
mod tests;
