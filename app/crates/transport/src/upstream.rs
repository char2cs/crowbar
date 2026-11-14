use http::header::AUTHORIZATION;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::protocol::WebSocketConfig;
use tokio_tungstenite::{WebSocketStream, client_async_with_config};

use crate::async_stream::BoxedStream;
use crate::dial::{DIAL_TIMEOUT, dial};
use crate::error::Error;
use crate::host_target::HostTarget;
use crate::remote::paired_host;
use crate::transport::Transport;

/// A daemon-side WebSocket over whichever transport the target needs.
pub type UpstreamSocket = WebSocketStream<BoxedStream>;

pub(crate) fn ws_config(max_message: usize) -> WebSocketConfig {
    WebSocketConfig::default()
        .max_message_size(Some(max_message))
        .max_frame_size(Some(max_message))
}

async fn connect(
    transport: &Transport,
    target: &HostTarget,
    request: &mut http::Request<()>,
) -> Result<BoxedStream, Error> {
    match target {
        HostTarget::Local => {
            let socket = crate::local::connect_once(transport.locator()).await?;
            Ok(Box::new(socket))
        },
        HostTarget::Remote(id) => {
            let paired = paired_host(transport.registry(), transport.tokens(), id).await?;
            request
                .headers_mut()
                .insert(AUTHORIZATION, paired.authorization);
            Ok(Box::new(dial(&paired.host).await?))
        },
    }
}

pub(crate) async fn open(
    transport: &Transport,
    target: &HostTarget,
    path_and_query: &str,
    max_message: usize,
) -> Result<UpstreamSocket, Error> {
    let mut request = format!("ws://localhost{path_and_query}")
        .into_client_request()
        .map_err(|e| Error::WebSocket(Box::new(e)))?;
    let stream = connect(transport, target, &mut request).await?;
    let upgrade = client_async_with_config(request, stream, Some(ws_config(max_message)));
    let upgraded = match target {
        HostTarget::Local => upgrade.await,
        HostTarget::Remote(_) => tokio::time::timeout(DIAL_TIMEOUT, upgrade)
            .await
            .map_err(|_| Error::UpgradeTimeout)?,
    };
    Ok(upgraded.map_err(|e| Error::WebSocket(Box::new(e)))?.0)
}

#[cfg(test)]
#[path = "upstream_tests.rs"]
mod tests;
