use std::path::PathBuf;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Arc;
use std::time::Duration;

use tauri::http;

use tokio::net::UnixListener;

use super::test_support::{serve_once_unix, tempdir};
use super::{
    connect_retry_delay, forward, readable_by, remote_host_id, serve, CONNECT_ATTEMPTS,
    DAEMON_UNAVAILABLE, PROXY_HEADER,
};

const PAGE: &str = "tauri://localhost";

fn request(method: http::Method) -> http::Request<Vec<u8>> {
    http::Request::builder()
        .method(method)
        .uri("crowbar://localhost/v0/projects")
        .body(Vec::new())
        .unwrap()
}

fn socket_in(dir: &tempdir::Dir) -> PathBuf {
    dir.path.join("s.sock")
}

#[test]
fn connect_retry_delay_polls_fast_then_backs_off_within_the_window() {
    assert_eq!(connect_retry_delay(1), Duration::from_millis(25));
    assert_eq!(connect_retry_delay(40), Duration::from_millis(25));
    assert_eq!(connect_retry_delay(41), Duration::from_millis(100));
    assert_eq!(connect_retry_delay(44), Duration::from_millis(800));
    assert_eq!(connect_retry_delay(45), Duration::from_secs(1));
    let total: Duration = (1..CONNECT_ATTEMPTS).map(connect_retry_delay).sum();
    assert_eq!(total, Duration::from_millis(4500));
}

// Regression: a GET issued while the daemon is still binding (or mid-respawn)
// must reach it once it listens, instead of answering the webview a 502 it
// treats as terminal.
#[tokio::test]
async fn get_retries_the_connect_until_the_daemon_listens() {
    let _serialised = crate::test_support::fd_tests().await;
    let dir = tempdir::Dir::new("retry");
    let path = socket_in(&dir);
    let waits = Arc::new(AtomicU32::new(0));
    let resp = {
        let bind_path = path.clone();
        let waits = waits.clone();
        forward(
            || Some(path.clone()),
            request(http::Method::GET),
            move |_attempt| {
                // The first retry IS the signal the daemon binds on.
                if waits.fetch_add(1, Ordering::SeqCst) == 0 {
                    serve_once_unix(bind_path.clone());
                }
                async {}
            },
        )
        .await
    };
    assert_eq!(resp.status(), 200);
    assert_eq!(resp.body(), b"ok");
    assert_eq!(waits.load(Ordering::SeqCst), 1);
}

#[tokio::test]
async fn browser_webviews_never_reach_the_daemon() {
    let _serialised = crate::test_support::fd_tests().await;
    let dir = tempdir::Dir::new("browser-label");
    let path = socket_in(&dir);
    let listener = UnixListener::bind(&path).expect("bind");
    let resp = serve(
        "browser-abc",
        forward(
            || Some(path.clone()),
            request(http::Method::GET),
            |_| async {},
        ),
    )
    .await;
    assert_eq!(resp.status(), 403);
    // The connect is already complete or never happened by now, so a pending accept means no connection.
    assert!(
        futures_util::FutureExt::now_or_never(listener.accept()).is_none(),
        "a browser tab's request reached the daemon socket"
    );
}

#[tokio::test]
async fn browser_webviews_never_reach_a_remote_host() {
    let resp = serve("browser-abc", async {
        panic!("a browser tab's request was routed to a host")
    })
    .await;
    assert_eq!(resp.status(), 403);
}

#[tokio::test]
async fn the_main_webview_still_reaches_the_daemon() {
    let _serialised = crate::test_support::fd_tests().await;
    let dir = tempdir::Dir::new("main-label");
    let path = socket_in(&dir);
    serve_once_unix(path.clone());
    let resp = serve(
        "main",
        forward(
            || Some(path.clone()),
            request(http::Method::GET),
            |_| async {},
        ),
    )
    .await;
    assert_eq!(resp.status(), 200);
}

#[tokio::test]
async fn get_gives_up_after_the_budget_with_a_marked_503() {
    let _serialised = crate::test_support::fd_tests().await;
    let dir = tempdir::Dir::new("budget");
    let path = socket_in(&dir);
    let waits = Arc::new(AtomicU32::new(0));
    let resp = {
        let waits = waits.clone();
        forward(
            || Some(path.clone()),
            request(http::Method::GET),
            move |_attempt| {
                waits.fetch_add(1, Ordering::SeqCst);
                async {}
            },
        )
        .await
    };
    assert_eq!(resp.status(), 503);
    assert_eq!(
        resp.headers()
            .get(PROXY_HEADER)
            .map(|v| v.to_str().unwrap()),
        Some(DAEMON_UNAVAILABLE)
    );
    assert_eq!(waits.load(Ordering::SeqCst), CONNECT_ATTEMPTS - 1);
}

#[tokio::test]
async fn mutation_is_never_replayed_and_is_marked_unavailable() {
    let resp = forward(
        || None,
        request(http::Method::POST),
        |_attempt| async { panic!("a POST must not be retried") },
    )
    .await;
    assert_eq!(resp.status(), 503);
    assert_eq!(
        resp.headers()
            .get(PROXY_HEADER)
            .map(|v| v.to_str().unwrap()),
        Some(DAEMON_UNAVAILABLE)
    );
}

#[tokio::test]
async fn socket_published_after_the_first_attempt_is_picked_up() {
    let _serialised = crate::test_support::fd_tests().await;
    let dir = tempdir::Dir::new("late");
    let path = socket_in(&dir);
    let published = Arc::new(std::sync::Mutex::new(None::<PathBuf>));
    let resp = {
        let published_reader = published.clone();
        let bind_path = path.clone();
        forward(
            move || published_reader.lock().unwrap().clone(),
            request(http::Method::GET),
            move |_attempt| {
                serve_once_unix(bind_path.clone());
                published.lock().unwrap().replace(bind_path.clone());
                async {}
            },
        )
        .await
    };
    assert_eq!(resp.status(), 200);
}

fn cors(resp: &http::Response<Vec<u8>>, name: http::header::HeaderName) -> Option<&str> {
    resp.headers().get(name).map(|v| v.to_str().unwrap())
}

// Regression: without these, WebKit rejected the fetch with an opaque
// "Load failed" and the marker never reached the page.
#[tokio::test]
async fn a_proxy_answer_is_readable_by_the_page_that_asked() {
    let resp = forward(|| None, request(http::Method::POST), |_| async {}).await;

    let resp = readable_by(Some(http::HeaderValue::from_static(PAGE)), resp);

    assert_eq!(resp.status(), 503);
    let allow_origin = http::header::ACCESS_CONTROL_ALLOW_ORIGIN;
    assert_eq!(cors(&resp, allow_origin), Some(PAGE));
    let exposed = cors(&resp, http::header::ACCESS_CONTROL_EXPOSE_HEADERS);
    assert_eq!(exposed, Some(PROXY_HEADER));
    assert_eq!(cors(&resp, http::header::VARY), Some("Origin"));
    assert_eq!(
        cors(&resp, PROXY_HEADER.parse().unwrap()),
        Some(DAEMON_UNAVAILABLE)
    );
}

#[tokio::test]
async fn a_daemon_answer_keeps_its_own_cors_decision() {
    let _serialised = crate::test_support::fd_tests().await;
    let dir = tempdir::Dir::new("cors");
    let path = socket_in(&dir);
    serve_once_unix(path.clone());

    let resp = forward(
        || Some(path.clone()),
        request(http::Method::GET),
        |_| async {},
    )
    .await;
    let resp = readable_by(Some(http::HeaderValue::from_static(PAGE)), resp);

    assert_eq!(resp.status(), 200);
    let allow_origin = http::header::ACCESS_CONTROL_ALLOW_ORIGIN;
    assert_eq!(
        cors(&resp, allow_origin),
        None,
        "the daemon did not grant it"
    );
}

#[test]
fn only_a_non_localhost_authority_routes_to_a_remote_host() {
    let host = |uri: &'static str| remote_host_id(&uri.parse().unwrap()).map(str::to_string);
    assert_eq!(host("crowbar://localhost/v0/projects"), None);
    assert_eq!(host("crowbar://localhost:80/v0/projects"), None);
    assert_eq!(host("crowbar://host-1/v0/projects"), Some("host-1".into()));
}
