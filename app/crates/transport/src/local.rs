use std::future::Future;
use std::time::Duration;

use http::{Method, Request, Response};
use tokio::net::UnixStream;

use crate::error::Error;
use crate::http_conn::exchange;
use crate::response::{DAEMON_UNAVAILABLE, error_response, marked_response};
use crate::socket_locator::SocketLocator;

/// Connect attempts (~4.5s in all) for an idempotent read that cannot reach the daemon: the
/// socket path is published before the daemon binds, and the pane only retries rejected
/// fetches, not a 502. Dense 25ms polls first, then an exponential tail for a slow start.
pub const CONNECT_ATTEMPTS: u32 = CONNECT_FAST_STEPS + 7;
const CONNECT_FAST_STEPS: u32 = 40;
const CONNECT_FAST_DELAY: Duration = Duration::from_millis(25);
const CONNECT_RETRY_BASE: Duration = Duration::from_millis(100);
const CONNECT_RETRY_CAP: Duration = Duration::from_secs(1);
const MAX_BACKOFF_STEPS: u32 = 8;

/// How long to wait after failed connect attempt number `attempt` (counting from 1).
#[must_use]
pub fn connect_retry_delay(attempt: u32) -> Duration {
    if attempt <= CONNECT_FAST_STEPS {
        return CONNECT_FAST_DELAY;
    }
    let tail = (attempt - CONNECT_FAST_STEPS - 1).min(MAX_BACKOFF_STEPS);
    (CONNECT_RETRY_BASE * 2u32.saturating_pow(tail)).min(CONNECT_RETRY_CAP)
}

fn is_idempotent_read(method: &Method) -> bool {
    matches!(*method, Method::GET | Method::HEAD)
}

async fn try_connect(locator: &dyn SocketLocator) -> Result<UnixStream, String> {
    let Some(path) = locator.socket_path() else {
        return Err("crowbar daemon socket not ready".to_owned());
    };
    UnixStream::connect(&path)
        .await
        .map_err(|e| format!("crowbar daemon socket {}: {e}", path.display()))
}

/// Connects to the daemon once, re-reading the socket path.
///
/// # Errors
/// [`Error::LocalSocket`] when the daemon cannot be dialled.
pub async fn connect_once(locator: &dyn SocketLocator) -> Result<UnixStream, Error> {
    try_connect(locator).await.map_err(Error::LocalSocket)
}

/// Connects to the daemon, waiting through `wait` between attempts and re-reading the socket
/// path each time. Only an idempotent caller may retry: a failure at connect proves the request
/// never reached the daemon.
///
/// # Errors
/// [`Error::LocalSocket`] with the last failure once attempts are spent.
pub async fn connect<W, F>(locator: &dyn SocketLocator, mut wait: W) -> Result<UnixStream, Error>
where
    W: FnMut(u32) -> F,
    F: Future<Output = ()>,
{
    let mut attempt = 1;
    loop {
        let failure = match try_connect(locator).await {
            Ok(stream) => return Ok(stream),
            Err(failure) => failure,
        };
        if attempt >= CONNECT_ATTEMPTS {
            return Err(Error::LocalSocket(failure));
        }
        wait(attempt).await;
        attempt += 1;
    }
}

/// Relays `request` to the local daemon, retrying the connect for idempotent reads. The local
/// daemon is this app's own, so its answer is not size-capped.
pub async fn forward_local<W, F>(
    locator: &dyn SocketLocator,
    request: Request<Vec<u8>>,
    wait: W,
) -> Response<Vec<u8>>
where
    W: FnMut(u32) -> F,
    F: Future<Output = ()>,
{
    let connected = if is_idempotent_read(request.method()) {
        connect(locator, wait).await
    } else {
        connect_once(locator).await
    };
    let stream = match connected {
        Ok(stream) => stream,
        Err(e) => return marked_response(503, DAEMON_UNAVAILABLE, &e.to_string()),
    };
    match exchange(Box::new(stream), request, usize::MAX).await {
        Ok(response) => response,
        Err(e) => error_response(502, &format!("crowbar proxy error: {e}")),
    }
}

#[cfg(test)]
#[path = "local_tests.rs"]
mod tests;
