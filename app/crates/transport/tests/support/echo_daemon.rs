use std::path::PathBuf;
use std::sync::{Arc, Mutex};

use futures_util::{SinkExt, StreamExt};
use tokio::net::{UnixListener, UnixStream};
use tokio_tungstenite::tungstenite::Message;
use tokio_tungstenite::tungstenite::handshake::server::{Request, Response};

/// A fake daemon that upgrades every connection to a WebSocket, records the path it was dialled
/// on, and echoes frames back with their kind intact.
pub(crate) struct EchoDaemon {
    pub(crate) socket: PathBuf,
    pub(crate) paths: Arc<Mutex<Vec<String>>>,
    _dir: tempfile::TempDir,
}

impl EchoDaemon {
    pub(crate) fn start() -> Self {
        let dir = tempfile::tempdir().expect("temp dir");
        let socket = dir.path().join("echo.sock");
        let listener = UnixListener::bind(&socket).expect("bind echo daemon");
        let paths = Arc::new(Mutex::new(Vec::new()));
        tokio::spawn(accept(listener, Arc::clone(&paths)));
        Self {
            socket,
            paths,
            _dir: dir,
        }
    }

    pub(crate) fn dialled(&self) -> Vec<String> {
        self.paths.lock().expect("paths lock").clone()
    }
}

async fn accept(
    listener: UnixListener,
    paths: Arc<Mutex<Vec<String>>>,
) {
    loop {
        let Ok((stream, _)) = listener.accept().await else {
            return;
        };
        tokio::spawn(echo(stream, Arc::clone(&paths)));
    }
}

async fn echo(
    stream: UnixStream,
    paths: Arc<Mutex<Vec<String>>>,
) {
    let record = |request: &Request, response: Response| {
        if let Ok(mut seen) = paths.lock() {
            seen.push(request.uri().to_string());
        }
        Ok(response)
    };
    let Ok(ws) = tokio_tungstenite::accept_hdr_async_with_config(stream, record, Some(big())).await
    else {
        return;
    };
    let (mut write, mut read) = ws.split();
    while let Some(Ok(message)) = read.next().await {
        let closing = message.is_close();
        if write.send(message).await.is_err() || closing {
            return;
        }
    }
}

fn big() -> tokio_tungstenite::tungstenite::protocol::WebSocketConfig {
    tokio_tungstenite::tungstenite::protocol::WebSocketConfig::default()
        .max_message_size(None)
        .max_frame_size(None)
}

pub(crate) fn text(s: &str) -> Message {
    Message::Text(s.into())
}
