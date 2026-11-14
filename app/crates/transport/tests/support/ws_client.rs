use tokio::net::TcpStream;
use tokio_tungstenite::tungstenite::Error as WsError;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::http::header::{HeaderValue, ORIGIN, SEC_WEBSOCKET_PROTOCOL};
use tokio_tungstenite::tungstenite::protocol::WebSocketConfig;
use tokio_tungstenite::{MaybeTlsStream, WebSocketStream, client_async_with_config};
use transport::{RELAY_SUBPROTOCOL, TOKEN_SUBPROTOCOL_PREFIX};

pub(crate) type Client = WebSocketStream<MaybeTlsStream<TcpStream>>;

/// How a pane's `WebSocket` dials the relay; each field can be bent to break one check.
pub(crate) struct Dial {
    pub(crate) port: u16,
    pub(crate) host: Option<String>,
    pub(crate) origins: Vec<String>,
    pub(crate) protocols: Vec<String>,
    pub(crate) path: String,
}

impl Dial {
    pub(crate) fn good(port: u16, origin: &str, token: &str, path: &str) -> Self {
        Self {
            port,
            host: None,
            origins: vec![origin.to_owned()],
            protocols: vec![
                RELAY_SUBPROTOCOL.to_owned(),
                format!("{TOKEN_SUBPROTOCOL_PREFIX}{token}"),
            ],
            path: path.to_owned(),
        }
    }

    pub(crate) async fn connect(self) -> Result<Client, WsError> {
        let mut request =
            format!("ws://127.0.0.1:{}{}", self.port, self.path).into_client_request()?;
        let headers = request.headers_mut();
        if let Some(host) = &self.host {
            headers.insert("host", header(host));
        }
        for origin in &self.origins {
            headers.append(ORIGIN, header(origin));
        }
        if !self.protocols.is_empty() {
            headers.insert(SEC_WEBSOCKET_PROTOCOL, header(&self.protocols.join(", ")));
        }
        let tcp = TcpStream::connect(("127.0.0.1", self.port))
            .await
            .map_err(WsError::Io)?;
        let config = WebSocketConfig::default()
            .max_message_size(None)
            .max_frame_size(None);
        let stream = MaybeTlsStream::Plain(tcp);
        client_async_with_config(request, stream, Some(config))
            .await
            .map(|(ws, _)| ws)
    }
}

fn header(value: &str) -> HeaderValue {
    HeaderValue::from_str(value).unwrap_or_else(|_| HeaderValue::from_static("invalid"))
}

pub(crate) fn status_of(error: &WsError) -> Option<u16> {
    match error {
        WsError::Http(response) => Some(response.status().as_u16()),
        _ => None,
    }
}
