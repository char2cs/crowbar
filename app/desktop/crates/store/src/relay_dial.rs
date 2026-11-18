//! Opening the daemon's stream through the shell's loopback relay.

use std::sync::Arc;

use futures_util::future::ready;
use futures_util::stream::{SplitSink, SplitStream};
use futures_util::{SinkExt, StreamExt};
use protocol::{DaemonTransport, HostId, PaneId, PaneOrigin, RelayScope};
use tokio::io::{AsyncRead, AsyncWrite};
use tokio::net::TcpStream;
use tokio::sync::mpsc;
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::client::IntoClientRequest;
use tokio_tungstenite::tungstenite::http::HeaderValue;
use tokio_tungstenite::{WebSocketStream, client_async};

use crate::connection::Connection;
use crate::dial::{Dial, Opening};
use crate::error::{Error, Result};

const RELAY_SUBPROTOCOL: &str = "crowbar.relay.v1";
const TOKEN_PREFIX: &str = "crowbar.token.";
const STREAM_PATH: &str = "/v0/stream";
const QUEUE: usize = 1024;

pub(crate) struct RelayDial {
    transport: Arc<dyn DaemonTransport>,
    host: HostId,
    origin: String,
}

impl RelayDial {
    pub(crate) fn new(
        transport: Arc<dyn DaemonTransport>,
        host: HostId,
        origin: &str,
    ) -> Self {
        Self {
            transport,
            host,
            origin: origin.to_owned(),
        }
    }

    async fn connect(&self) -> Result<Connection> {
        let pane = PaneId::new("shell-sidebar").map_err(stream_error)?;
        let scope = RelayScope::new(self.host.clone(), pane, PaneOrigin::Core);
        let endpoint = self
            .transport
            .relay_endpoint(&scope)
            .map_err(stream_error)?;
        let address = endpoint.url().trim_start_matches("ws://").to_owned();
        let tcp = TcpStream::connect(&address)
            .await
            .map_err(|source| Error::Stream(format!("connect {address}: {source}")))?;
        let url = format!("{}{STREAM_PATH}", endpoint.url());
        handshake(tcp, (&url, &self.origin), endpoint.token()).await
    }
}

impl Dial for RelayDial {
    fn open(&self) -> Opening<'_> {
        Box::pin(self.connect())
    }
}

pub(crate) async fn handshake<S>(
    io: S,
    (url, origin): (&str, &str),
    token: &str,
) -> Result<Connection>
where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    let mut request = url.into_client_request().map_err(stream_error)?;
    let protocols = format!("{RELAY_SUBPROTOCOL}, {TOKEN_PREFIX}{token}");
    let headers = request.headers_mut();
    headers.insert("sec-websocket-protocol", header(&protocols)?);
    headers.insert("origin", header(origin)?);
    let (socket, _) = client_async(request, io).await.map_err(stream_error)?;
    Ok(pump(socket))
}

fn header(value: &str) -> Result<HeaderValue> {
    HeaderValue::from_str(value).map_err(stream_error)
}

fn pump<S>(socket: WebSocketStream<S>) -> Connection
where
    S: AsyncRead + AsyncWrite + Unpin + Send + 'static,
{
    let (sink, stream) = socket.split();
    let (frames, incoming) = mpsc::channel(QUEUE);
    let (outgoing, requests) = mpsc::unbounded_channel::<String>();
    tokio::spawn(forward_frames(stream, frames));
    tokio::spawn(forward_requests(sink, requests));
    Connection { incoming, outgoing }
}

async fn forward_frames<S>(
    stream: SplitStream<WebSocketStream<S>>,
    frames: mpsc::Sender<String>,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    let mut texts = stream.filter_map(|message| ready(text_of(message)));
    while let Some(text) = texts.next().await
        && frames.send(text).await.is_ok()
    {}
}

async fn forward_requests<S>(
    mut sink: SplitSink<WebSocketStream<S>, Message>,
    mut requests: mpsc::UnboundedReceiver<String>,
) where
    S: AsyncRead + AsyncWrite + Unpin,
{
    while let Some(text) = requests.recv().await
        && sink.send(Message::text(text)).await.is_ok()
    {}
}

fn text_of(
    message: std::result::Result<Message, tokio_tungstenite::tungstenite::Error>
) -> Option<String> {
    match message {
        Ok(Message::Text(text)) => Some(text.as_str().to_owned()),
        _ => None,
    }
}

fn stream_error(error: impl std::fmt::Display) -> Error {
    Error::Stream(error.to_string())
}

#[cfg(test)]
mod tests {
    use futures_util::{SinkExt, StreamExt};
    use tokio::io::{DuplexStream, duplex};
    use tokio_tungstenite::tungstenite::Message;
    use tokio_tungstenite::tungstenite::handshake::server::{
        Callback, ErrorResponse, Request, Response,
    };

    use super::*;

    type Seen = (Option<HeaderValue>, Option<HeaderValue>);

    struct Record<'a>(&'a mut Option<Seen>);

    impl Callback for Record<'_> {
        fn on_request(
            self,
            request: &Request,
            mut response: Response,
        ) -> std::result::Result<Response, ErrorResponse> {
            *self.0 = Some((
                request.headers().get("origin").cloned(),
                request.headers().get("sec-websocket-protocol").cloned(),
            ));
            response.headers_mut().insert(
                "sec-websocket-protocol",
                HeaderValue::from_static(RELAY_SUBPROTOCOL),
            );
            Ok(response)
        }
    }

    async fn serve(io: DuplexStream) -> (Option<Seen>, Message) {
        let mut seen = None;
        let mut socket = tokio_tungstenite::accept_hdr_async(io, Record(&mut seen))
            .await
            .unwrap();
        socket
            .send(Message::text(r#"{"t":"ready","epoch":"e"}"#))
            .await
            .unwrap();
        let asked = socket.next().await.unwrap().unwrap();
        (seen, asked)
    }

    #[tokio::test]
    async fn the_handshake_offers_the_relay_subprotocols_and_pumps_text_both_ways() {
        let (client_io, server_io) = duplex(8192);
        let server = tokio::spawn(serve(server_io));
        let mut connection = handshake(
            client_io,
            ("ws://127.0.0.1:1/v0/stream", "crowbar://local"),
            "tok",
        )
        .await
        .unwrap();
        assert_eq!(
            connection.incoming.recv().await.as_deref(),
            Some(r#"{"t":"ready","epoch":"e"}"#)
        );
        connection
            .outgoing
            .send(r#"{"op":"sub","scope":"projects"}"#.into())
            .unwrap();
        let (seen, asked) = server.await.unwrap();
        let (origin, protocols) = seen.unwrap();
        assert_eq!(origin.unwrap(), "crowbar://local");
        assert_eq!(protocols.unwrap(), "crowbar.relay.v1, crowbar.token.tok");
        assert_eq!(
            asked.into_text().unwrap().as_str(),
            r#"{"op":"sub","scope":"projects"}"#
        );
    }

    #[tokio::test]
    async fn a_server_that_is_not_a_websocket_is_a_stream_error() {
        let (client_io, server_io) = duplex(1024);
        drop(server_io);
        let failed = handshake(
            client_io,
            ("ws://127.0.0.1:1/v0/stream", "crowbar://local"),
            "t",
        )
        .await;
        assert!(matches!(failed, Err(Error::Stream(_))));
    }
}
