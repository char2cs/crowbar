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

async fn serve(state: &RelayState, client: WebSocketStream<TcpStream>, admission: Admission) {
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
#[path = "session_tests.rs"]
mod tests;
