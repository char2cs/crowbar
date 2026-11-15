use tokio::io::{AsyncRead, AsyncWrite};
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::handshake::client::Request;
use tokio_tungstenite::{client_async, WebSocketStream};

pub(super) fn upgrade_request(path: &str) -> Result<Request, String> {
    format!("ws://localhost{path}")
        .into_client_request()
        .map_err(|e| format!("build ws request: {e}"))
}

pub(super) async fn upgrade<T>(
    stream: T,
    request: Request,
) -> Result<WebSocketStream<T>, String>
where
    T: AsyncRead + AsyncWrite + Unpin,
{
    let (ws, _response) = client_async(request, stream).await.map_err(|e| {
        log::error!("ws_open: ws upgrade failed: {e}");
        format!("ws upgrade: {e}")
    })?;
    Ok(ws)
}

#[cfg(test)]
mod tests {
    use super::*;
    use tokio::io::duplex;

    #[test]
    fn the_request_targets_the_path_on_a_placeholder_host() {
        const CASES: &[(&str, Option<&str>)] = &[
            ("/v0/x", Some("ws://localhost/v0/x")),
            ("/v0/a?b=1", Some("ws://localhost/v0/a?b=1")),
            ("/v0/a b", None),
        ];
        for (path, expected) in CASES {
            let built = upgrade_request(path).ok().map(|r| r.uri().to_string());
            assert_eq!(built.as_deref(), *expected, "{path}");
        }
    }

    #[tokio::test]
    async fn a_peer_that_never_speaks_websocket_fails_the_upgrade() {
        let (client, server) = duplex(1024);
        drop(server);
        let request = upgrade_request("/v0/x").unwrap();
        let error = upgrade(client, request).await.expect_err("no handshake");
        assert!(error.starts_with("ws upgrade: "), "{error}");
    }
}
