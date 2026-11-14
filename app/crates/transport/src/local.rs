use std::future::Future;
use std::time::Duration;

use http::{Method, Request, Response};
use tokio::net::UnixStream;

use crate::error::Error;
use crate::http_conn::exchange;
use crate::response::{DAEMON_UNAVAILABLE, error_response, marked_response};
use crate::socket_locator::SocketLocator;

pub(crate) const CONNECT_ATTEMPTS: u32 = CONNECT_FAST_STEPS + 7;
const CONNECT_FAST_STEPS: u32 = 40;
const CONNECT_FAST_DELAY: Duration = Duration::from_millis(25);
const CONNECT_RETRY_BASE: Duration = Duration::from_millis(100);
const CONNECT_RETRY_CAP: Duration = Duration::from_secs(1);
const MAX_BACKOFF_STEPS: u32 = 8;
const LOCAL_BODY_LIMIT: usize = 1024 * 1024 * 1024;
const UNREACHABLE_TEXT: &str = "the crowbar daemon is not reachable";

pub(crate) fn connect_retry_delay(attempt: u32) -> Duration {
    if attempt <= CONNECT_FAST_STEPS {
        return CONNECT_FAST_DELAY;
    }
    let tail = (attempt - CONNECT_FAST_STEPS - 1).min(MAX_BACKOFF_STEPS);
    (CONNECT_RETRY_BASE * 2u32.saturating_pow(tail)).min(CONNECT_RETRY_CAP)
}

fn is_idempotent_read(method: &Method) -> bool {
    matches!(*method, Method::GET | Method::HEAD)
}

pub(crate) async fn connect_once(locator: &dyn SocketLocator) -> Result<UnixStream, Error> {
    let path = locator.socket_path().ok_or(Error::NoSocket)?;
    UnixStream::connect(&path).await.map_err(|e| {
        tracing::debug!("daemon socket {} not connectable: {e}", path.display());
        Error::LocalSocket(e)
    })
}

pub(crate) async fn connect<W, F>(
    locator: &dyn SocketLocator,
    mut wait: W,
) -> Result<UnixStream, Error>
where
    W: FnMut(u32) -> F,
    F: Future<Output = ()>,
{
    let mut attempt = 1;
    loop {
        match connect_once(locator).await {
            Ok(stream) => return Ok(stream),
            Err(failure) if attempt >= CONNECT_ATTEMPTS => return Err(failure),
            Err(_) => {},
        }
        wait(attempt).await;
        attempt += 1;
    }
}

pub(crate) async fn forward_local<W, F>(
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
        Err(e) => {
            tracing::debug!("local daemon unreachable: {e}");
            return marked_response(503, DAEMON_UNAVAILABLE, UNREACHABLE_TEXT);
        },
    };
    match exchange(Box::new(stream), request, LOCAL_BODY_LIMIT).await {
        Ok(response) => response,
        Err(Error::TooLarge) => error_response(502, "local response too large"),
        Err(e) => {
            tracing::debug!("local daemon exchange failed: {e}");
            error_response(502, "crowbar proxy error")
        },
    }
}

#[cfg(test)]
#[path = "local_tests.rs"]
mod tests;
