use std::future::Future;
use std::time::Duration;

use http::{Method, Request, StatusCode};
use tokio::net::UnixStream;

use crate::error::Error;
use crate::http_conn::exchange;
use crate::response::{
    DAEMON_UNAVAILABLE, Forwarded, error_response, marked_response, proxy_answer,
};
use crate::socket_locator::SocketLocator;
use crate::socket_trust::verify;

pub(crate) const CONNECT_ATTEMPTS: u32 = CONNECT_FAST_STEPS + 7;
const CONNECT_FAST_STEPS: u32 = 40;
const CONNECT_FAST_DELAY: Duration = Duration::from_millis(25);
const CONNECT_RETRY_BASE: Duration = Duration::from_millis(100);
const CONNECT_RETRY_CAP: Duration = Duration::from_secs(1);
const MAX_BACKOFF_STEPS: u32 = 8;
const LOCAL_BODY_LIMIT: usize = 64 * 1024 * 1024;
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
    let stream = UnixStream::connect(&path).await.map_err(|e| {
        tracing::debug!("daemon socket {} not connectable: {e}", path.display());
        Error::LocalSocket(e)
    })?;
    verify(&path, &stream)?;
    Ok(stream)
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
            Err(failure) => log_retry(attempt, &failure),
        }
        wait(attempt).await;
        attempt += 1;
    }
}

fn log_retry(
    attempt: u32,
    failure: &Error,
) {
    tracing::debug!("daemon socket not ready (attempt {attempt}): {failure}");
}

async fn connect_for<W, F>(
    method: &Method,
    locator: &dyn SocketLocator,
    wait: W,
) -> Result<UnixStream, Error>
where
    W: FnMut(u32) -> F,
    F: Future<Output = ()>,
{
    if is_idempotent_read(method) {
        return connect(locator, wait).await;
    }
    connect_once(locator).await
}

pub(crate) async fn forward_local<W, F>(
    locator: &dyn SocketLocator,
    request: Request<Vec<u8>>,
    wait: W,
) -> Forwarded
where
    W: FnMut(u32) -> F,
    F: Future<Output = ()>,
{
    forward_limited(locator, request, wait, LOCAL_BODY_LIMIT).await
}

async fn forward_limited<W, F>(
    locator: &dyn SocketLocator,
    request: Request<Vec<u8>>,
    wait: W,
    limit: usize,
) -> Forwarded
where
    W: FnMut(u32) -> F,
    F: Future<Output = ()>,
{
    let stream = match connect_for(request.method(), locator, wait).await {
        Ok(stream) => stream,
        Err(e) => {
            tracing::debug!("local daemon unreachable: {e}");
            return proxy_answer(marked_response(
                StatusCode::SERVICE_UNAVAILABLE,
                DAEMON_UNAVAILABLE,
                UNREACHABLE_TEXT,
            ));
        },
    };
    match exchange(Box::new(stream), request, limit).await {
        Ok(response) => Ok(response),
        Err(Error::TooLarge) => proxy_answer(error_response(
            StatusCode::BAD_GATEWAY,
            "local response too large",
        )),
        Err(e) => {
            tracing::debug!("local daemon exchange failed: {e}");
            proxy_answer(error_response(
                StatusCode::BAD_GATEWAY,
                "crowbar proxy error",
            ))
        },
    }
}

#[cfg(test)]
mod tests {
    use std::path::PathBuf;
    use std::sync::atomic::{AtomicU32, Ordering};

    use super::*;
    use crate::response::marker_of;

    const MISSING_SOCKET: &str = "/crowbar-test-missing/d.sock";

    fn request(method: Method) -> Request<Vec<u8>> {
        Request::builder()
            .method(method)
            .uri("crowbar://localhost/v0/health")
            .body(Vec::new())
            .unwrap()
    }

    /// (attempt, the wait before the next one)
    const CASES: [(u32, Duration); 6] = [
        (1, Duration::from_millis(25)),
        (40, Duration::from_millis(25)),
        (41, Duration::from_millis(100)),
        (42, Duration::from_millis(200)),
        (46, Duration::from_secs(1)),
        (1000, Duration::from_secs(1)),
    ];

    #[test]
    #[ignore = "real-infra: a real unix listener answering one request"]
    fn retries_poll_densely_then_back_off_to_a_cap() {
        for (attempt, wait) in CASES {
            assert_eq!(connect_retry_delay(attempt), wait, "attempt {attempt}");
        }
    }

    fn counting(waits: &AtomicU32) -> impl Fn(u32) -> std::future::Ready<()> + '_ {
        |_| {
            waits.fetch_add(1, Ordering::SeqCst);
            std::future::ready(())
        }
    }

    #[tokio::test]
    #[ignore = "real-infra: a real unix listener answering one request"]
    async fn a_read_retries_until_the_attempts_are_spent_then_says_the_daemon_is_unavailable() {
        let waits = AtomicU32::new(0);
        let path = PathBuf::from(MISSING_SOCKET);

        let answer = forward_local(&path, request(Method::GET), counting(&waits))
            .await
            .unwrap_err();

        assert_eq!(answer.status(), 503);
        assert_eq!(marker_of(&answer), Some(DAEMON_UNAVAILABLE));
        assert_eq!(waits.load(Ordering::SeqCst), CONNECT_ATTEMPTS - 1);
        let body = String::from_utf8_lossy(answer.body());
        assert!(
            !body.contains(MISSING_SOCKET),
            "the page is not told where the socket is: {body}"
        );
    }

    #[tokio::test]
    #[ignore = "real-infra: a real unix listener answering one request"]
    async fn a_mutation_is_never_replayed() {
        let waits = AtomicU32::new(0);
        let path = PathBuf::from(MISSING_SOCKET);

        let answer = forward_local(&path, request(Method::POST), counting(&waits))
            .await
            .unwrap_err();

        assert_eq!(answer.status(), 503);
        assert_eq!(waits.load(Ordering::SeqCst), 0);
    }

    #[ignore = "real-infra: a real unix listener answering one request"]
    fn serve_ok_once(path: &std::path::Path) {
        let bound = tokio::net::UnixListener::bind(path).unwrap();
        tokio::spawn(answer_ok(bound));
    }

    #[ignore = "real-infra: a real unix listener answering one request"]
    async fn answer_ok(bound: tokio::net::UnixListener) {
        use tokio::io::{AsyncReadExt, AsyncWriteExt};

        let (mut stream, _) = bound.accept().await.unwrap();
        let mut buf = [0u8; 1024];
        let read = stream.read(&mut buf).await.unwrap();
        assert!(read > 0, "the request arrived");
        let reply = b"HTTP/1.1 200 OK\r\ncontent-length: 2\r\nconnection: close\r\n\r\nok";
        stream.write_all(reply).await.unwrap();
    }

    #[ignore = "real-infra: a real unix listener answering one request"]
    fn bind_on_third_attempt(
        path: &std::path::Path,
        attempt: u32,
    ) {
        if attempt == 3 {
            serve_ok_once(path);
        }
    }

    #[ignore = "real-infra: a real unix listener answering one request"]
    fn binding_late<'a>(
        attempts: &'a AtomicU32,
        path: &'a std::path::Path,
    ) -> impl Fn(u32) -> std::future::Ready<()> + 'a {
        |attempt| {
            attempts.store(attempt, Ordering::SeqCst);
            bind_on_third_attempt(path, attempt);
            std::future::ready(())
        }
    }

    #[tokio::test]
    #[ignore = "real-infra: dials a unix socket that a real listener binds"]
    async fn a_daemon_that_binds_late_is_reached_by_a_retried_read() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("d.sock");
        let listener_path = path.clone();
        let attempts = AtomicU32::new(0);

        let wait = binding_late(&attempts, &listener_path);
        let answer = forward_local(&path, request(Method::GET), wait)
            .await
            .unwrap();

        assert_eq!(answer.status(), 200);
        assert_eq!(answer.body(), b"ok");
        assert_eq!(attempts.load(Ordering::SeqCst), 3);
    }

    #[tokio::test]
    #[ignore = "real-infra: reads an over-limit answer from a real unix socket"]
    async fn a_response_over_the_limit_is_refused_with_a_clear_error() {
        let dir = tempfile::TempDir::new().unwrap();
        let path = dir.path().join("d.sock");
        serve_ok_once(&path);
        let answer = forward_limited(&path, request(Method::GET), counting(&AtomicU32::new(0)), 1)
            .await
            .unwrap_err();
        assert_eq!(answer.status(), 502);
        assert_eq!(answer.body(), b"local response too large");
    }
}
