use std::path::PathBuf;

use tokio::net::UnixStream;

use crate::dial::Dial;

pub(crate) struct SocketDial {
    path: PathBuf,
}

impl SocketDial {
    pub(crate) fn new(path: impl Into<PathBuf>) -> Self {
        Self { path: path.into() }
    }
}

impl Dial for SocketDial {
    type Stream = UnixStream;

    async fn dial(&self) -> std::io::Result<UnixStream> {
        UnixStream::connect(&self.path).await
    }
}

#[cfg(test)]
#[path = "socket_dial_tests.rs"]
mod tests;
