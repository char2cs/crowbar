use std::path::Path;

use crate::daemon_get::get;
use crate::error::Error;
use crate::socket_dial::SocketDial;

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
    use super::*;

    #[tokio::test]
    async fn a_missing_socket_is_a_dial_error() {
        let error = fetch(Path::new("/nonexistent/absent.sock"), "/v0/health", 16)
            .await
            .unwrap_err();
        assert!(matches!(error, Error::Dial { .. }), "{error:?}");
    }
}
