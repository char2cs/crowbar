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

fn upgrade_request(
    origin: &PaneOrigin,
    path_and_query: &str,
) -> Result<http::Request<()>, Error> {
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

impl Transport {
    pub(crate) async fn open_stream(
        &self,
        wanted: &UpstreamRequest,
    ) -> Result<UpstreamSocket, Error> {
        let mut request = upgrade_request(&wanted.origin, &wanted.path_and_query)?;
        let stream = connect(self, &wanted.target, &mut request).await?;
        let (socket, _response) =
            client_async_with_config(request, stream, Some(ws_config(wanted.max_message)))
                .await
                .map_err(Error::websocket)?;
        Ok(socket)
    }
}

#[cfg(test)]
mod tests {
    use protocol::ExtensionId;

    use super::*;
    use crate::access_scope::EXTENSION_HEADER;

    #[test]
    fn the_daemon_is_told_which_extension_opened_the_stream_and_core_streams_carry_nothing() {
        let acme = PaneOrigin::Extension(ExtensionId::new("acme").unwrap());
        let extension = upgrade_request(&acme, "/v0/ext/acme/stream?x=1").unwrap();
        assert_eq!(extension.headers()[EXTENSION_HEADER], "acme");
        assert_eq!(
            extension.uri().path_and_query().unwrap().as_str(),
            "/v0/ext/acme/stream?x=1"
        );
        let core = upgrade_request(&PaneOrigin::Core, "/v0/terminals/t1/stream").unwrap();
        assert!(core.headers().get(EXTENSION_HEADER).is_none());
    }

    #[test]
    fn an_unparsable_target_is_refused_before_anything_is_dialled() {
        assert!(upgrade_request(&PaneOrigin::Core, "/v0/a b").is_err());
    }
}
