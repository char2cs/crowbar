use std::path::{Path, PathBuf};

use tokio::net::UnixStream;

use crate::error::Error;
use crate::health::client::dial::Dial;
use crate::health::client::get::get;

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

/// Reads `target` (an absolute path and query, for example `/v0/health`) from the daemon
/// listening on `socket`, refusing an answer longer than `limit` bytes.
///
/// # Errors
/// [`Error::Dial`] when the socket cannot be reached, [`Error::Status`] for a non-success
/// status, [`Error::BodyTooLarge`] over the limit and [`Error::Exchange`] for a broken exchange.
pub async fn fetch(
    socket: &Path,
    target: &str,
    limit: usize,
) -> Result<Vec<u8>, Error> {
    get(&SocketDial::new(socket), target, limit).await
}

#[cfg(test)]
mod tests {
    use std::io::ErrorKind;

    use super::*;

    #[tokio::test]
    async fn a_missing_socket_is_a_dial_error_with_the_os_error() {
        let error = fetch(Path::new("/nonexistent/absent.sock"), "/v0/health", 16)
            .await
            .unwrap_err();
        assert!(
            matches!(&error, Error::Dial { source } if source.kind() == ErrorKind::NotFound),
            "{error:?}"
        );
    }
}
