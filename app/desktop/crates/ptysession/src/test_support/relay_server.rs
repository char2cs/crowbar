//! The relay's end of in-memory terminal streams.

use futures_util::{SinkExt as _, StreamExt as _};
use tokio::io::DuplexStream;
use tokio::sync::mpsc::UnboundedReceiver;
use tokio_tungstenite::accept_hdr_async;
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::handshake::server::{
    Callback, ErrorResponse, Request, Response,
};
use tokio_tungstenite::tungstenite::http::HeaderValue;

/// The server ends of dialled pipes.
pub(crate) type Accepted = UnboundedReceiver<DuplexStream>;

const RELAY_SUBPROTOCOL: &str = "crowbar.relay.v1";

struct SelectRelay {
    subprotocol: &'static str,
}

impl Callback for SelectRelay {
    fn on_request(
        self,
        _request: &Request,
        mut response: Response,
    ) -> Result<Response, ErrorResponse> {
        let selected = HeaderValue::from_static(self.subprotocol);
        response
            .headers_mut()
            .insert("sec-websocket-protocol", selected);
        Ok(response)
    }
}

const SELECT_RELAY: SelectRelay = SelectRelay {
    subprotocol: RELAY_SUBPROTOCOL,
};

/// Sends "ok" and an exit on the next stream, then closes it.
pub(crate) async fn serve_output(accepted: &mut Accepted) {
    let tcp = accepted.recv().await.unwrap();
    let mut socket = accept_hdr_async(tcp, SELECT_RELAY).await.unwrap();
    let output = Message::binary(vec![0, b'o', b'k']);
    socket.send(output).await.unwrap();
    let exit = Message::text(r#"{"type":"exit","code":0}"#);
    socket.send(exit).await.unwrap();
    socket.close(None).await.unwrap();
}

/// Closes the first stream at once, then serves the second as [`serve_output`].
pub(crate) async fn serve_then_drop(mut accepted: Accepted) {
    let tcp = accepted.recv().await.unwrap();
    let mut socket = accept_hdr_async(tcp, SELECT_RELAY).await.unwrap();
    socket.close(None).await.unwrap();
    serve_output(&mut accepted).await;
}

/// Sends "ok" and holds the stream until the client leaves.
pub(crate) async fn serve_held(mut accepted: Accepted) {
    let tcp = accepted.recv().await.unwrap();
    let mut socket = accept_hdr_async(tcp, SELECT_RELAY).await.unwrap();
    let output = Message::binary(vec![0, b'o', b'k']);
    socket.send(output).await.unwrap();
    while socket.next().await.is_some_and(|message| message.is_ok()) {}
}
