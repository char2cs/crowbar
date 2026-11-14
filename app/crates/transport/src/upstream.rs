use http::header::AUTHORIZATION;
use protocol::PaneOrigin;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::protocol::WebSocketConfig;
use tokio_tungstenite::{WebSocketStream, client_async_with_config};

use crate::access_scope::stamp;
use crate::async_stream::BoxedStream;
use crate::dial::dial;
use crate::error::Error;
use crate::host_target::HostTarget;
use crate::remote::paired_host;
use crate::transport::Transport;
use crate::upstream_request::UpstreamRequest;

pub(crate) type UpstreamSocket = WebSocketStream<BoxedStream>;

pub(crate) fn ws_config(max_message: usize) -> WebSocketConfig {
    WebSocketConfig::default()
        .max_message_size(Some(max_message))
        .max_frame_size(Some(max_message))
}

fn upgrade_request(origin: &PaneOrigin, path_and_query: &str) -> Result<http::Request<()>, Error> {
    let mut request = format!("ws://localhost{path_and_query}")
        .into_client_request()
        .map_err(Error::websocket)?;
    stamp(request.headers_mut(), origin);
    Ok(request)
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
    wanted: &UpstreamRequest,
) -> Result<UpstreamSocket, Error> {
    let mut request = upgrade_request(&wanted.origin, &wanted.path_and_query)?;
    let stream = connect(transport, &wanted.target, &mut request).await?;
    let (socket, _response) =
        client_async_with_config(request, stream, Some(ws_config(wanted.max_message)))
            .await
            .map_err(Error::websocket)?;
    Ok(socket)
}

#[cfg(test)]
#[path = "upstream_tests.rs"]
mod tests;
