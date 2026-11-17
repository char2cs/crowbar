//! Public-API tests of `transport` against real sockets and fake daemons.

#[path = "support/http_daemon.rs"]
mod http_daemon;

use std::sync::{Arc, Mutex};

use http::{Method, Request};
use protocol::ExtensionId;
use protocol::PaneOrigin;
use transport::{EXTENSION_HEADER, HostRegistry, MemoryTokenStore, Transport};

use crate::http_daemon::{Handler, Log, serve_http};

type TestResult = Result<(), Box<dyn std::error::Error>>;

#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
fn socket() -> std::io::Result<(tempfile::TempDir, std::path::PathBuf)> {
    let dir = tempfile::tempdir()?;
    let path = dir.path().join("daemon.sock");
    Ok((dir, path))
}

fn transport_at(path: std::path::PathBuf) -> Transport {
    Transport::new(
        Arc::new(path),
        Arc::new(HostRegistry::in_memory()),
        Arc::new(MemoryTokenStore::default()),
    )
}

#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
fn daemon(
    path: &std::path::Path,
    handler: Handler,
) -> std::io::Result<Log> {
    let listener = tokio::net::UnixListener::bind(path)?;
    let log: Log = Arc::new(Mutex::new(Vec::new()));
    let seen = Arc::clone(&log);
    tokio::spawn(accept_loop(listener, handler, seen));
    Ok(log)
}

#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
async fn accept_loop(
    listener: tokio::net::UnixListener,
    handler: Handler,
    seen: Log,
) {
    while let Ok((stream, _)) = listener.accept().await {
        tokio::spawn(serve_http(stream, Arc::clone(&handler), Arc::clone(&seen)));
    }
}

fn request(
    method: Method,
    uri: &str,
    body: &[u8],
) -> Request<Vec<u8>> {
    let mut built = Request::new(body.to_vec());
    *built.method_mut() = method;
    *built.uri_mut() = uri.parse().unwrap_or_default();
    built
}

fn ok(body: &str) -> Handler {
    let body = body.to_owned();
    Arc::new(move |_| {
        (
            200,
            vec![("content-type".into(), "application/json".into())],
            body.clone(),
        )
    })
}

#[tokio::test]
#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
async fn a_request_reaches_the_daemon_with_its_method_body_and_headers_and_the_answer_returns()
-> TestResult {
    let (_dir, path) = socket()?;
    let log = daemon(
        &path,
        Arc::new(|seen| {
            (
                201,
                vec![("x-seen".into(), seen.target.clone())],
                format!("got {}", seen.body.len()),
            )
        }),
    )?;
    let transport = transport_at(path);
    let mut req = request(
        Method::POST,
        "crowbar://localhost/v0/threads?limit=5",
        b"hello",
    );
    req.headers_mut().insert("x-pane", "p1".parse().unwrap());
    let answer = transport.handle(&PaneOrigin::Core, req).await;
    assert_eq!(answer.status(), 201);
    assert_eq!(answer.body(), b"got 5");
    assert_eq!(answer.headers()["x-seen"], "/v0/threads?limit=5");
    let seen = log.lock().unwrap()[0].clone();
    assert_eq!(
        (seen.method.as_str(), seen.body.as_slice()),
        ("POST", &b"hello"[..])
    );
    assert_eq!(seen.header("x-pane"), Some("p1"));
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
async fn the_extension_header_is_stamped_for_extensions_and_stripped_from_core_requests()
-> TestResult {
    let (_dir, path) = socket()?;
    let log = daemon(&path, ok("{}"))?;
    let transport = transport_at(path);
    let acme = PaneOrigin::Extension(ExtensionId::new("acme").unwrap());
    transport
        .handle(
            &acme,
            request(Method::GET, "crowbar://localhost/v0/ext/acme/x", b""),
        )
        .await;
    let mut forged = request(Method::GET, "crowbar://localhost/v0/projects", b"");
    forged
        .headers_mut()
        .insert(EXTENSION_HEADER, "victim".parse().unwrap());
    transport.handle(&PaneOrigin::Core, forged).await;
    let seen = log.lock().unwrap().clone();
    assert_eq!(seen[0].header(EXTENSION_HEADER), Some("acme"));
    assert_eq!(
        seen[1].header(EXTENSION_HEADER),
        None,
        "a pane cannot name an extension"
    );
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
async fn an_extension_cannot_reach_core_routes_and_the_daemon_never_hears_of_it() -> TestResult {
    let (_dir, path) = socket()?;
    let log = daemon(&path, ok("{}"))?;
    let transport = transport_at(path);
    let acme = PaneOrigin::Extension(ExtensionId::new("acme").unwrap());
    let cases = [
        "crowbar://localhost/v0/projects",
        "crowbar://localhost/v0/ext/other/x",
        "crowbar://localhost/v0/ext/acme/../projects",
        "crowbar://localhost/v0/ext/acme/%2e%2e/projects",
        "crowbar://localhost/v0/ext/acme//x",
        "crowbar://localhost/debug/pprof",
    ];
    for uri in cases {
        let answer = transport
            .handle(&acme, request(Method::GET, uri, b""))
            .await;
        assert_eq!(answer.status(), 403, "{uri}");
    }
    assert_eq!(log.lock().unwrap().len(), 0);
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
async fn a_daemon_supplied_proxy_marker_is_not_passed_to_the_page() -> TestResult {
    let (_dir, path) = socket()?;
    daemon(
        &path,
        Arc::new(|_| {
            (
                200,
                vec![("x-crowbar-proxy".into(), "host-identity-changed".into())],
                "{}".into(),
            )
        }),
    )?;
    let transport = transport_at(path);
    let answer = transport
        .handle(
            &PaneOrigin::Core,
            request(Method::GET, "crowbar://localhost/v0/x", b""),
        )
        .await;
    assert_eq!(transport::marker_of(&answer), None);
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
async fn cors_is_granted_only_to_answers_the_proxy_wrote() -> TestResult {
    let (_dir, path) = socket()?;
    daemon(&path, ok("{}"))?;
    let transport = transport_at(path.clone());
    let mut served = request(Method::GET, "crowbar://localhost/v0/x", b"");
    served
        .headers_mut()
        .insert("origin", "crowbar://app".parse().unwrap());
    let answer = transport.handle(&PaneOrigin::Core, served).await;
    assert!(
        answer
            .headers()
            .get("access-control-allow-origin")
            .is_none(),
        "the daemon grants its own"
    );

    std::fs::remove_file(&path).unwrap();
    let mut refused = request(Method::POST, "crowbar://localhost/v0/x", b"");
    refused
        .headers_mut()
        .insert("origin", "crowbar://app".parse().unwrap());
    let answer = transport.handle(&PaneOrigin::Core, refused).await;
    assert_eq!(answer.status(), 503);
    assert_eq!(transport::marker_of(&answer), Some("daemon-unavailable"));
    assert_eq!(
        answer.headers()["access-control-allow-origin"],
        "crowbar://app"
    );
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
async fn a_large_body_goes_through_intact_in_both_directions() -> TestResult {
    let (_dir, path) = socket()?;
    daemon(
        &path,
        Arc::new(|seen| {
            (
                200,
                vec![],
                String::from_utf8_lossy(&seen.body).into_owned(),
            )
        }),
    )?;
    let transport = transport_at(path);
    let payload = "x".repeat(4 * 1024 * 1024);
    let answer = transport
        .handle(
            &PaneOrigin::Core,
            request(Method::POST, "crowbar://localhost/v0/x", payload.as_bytes()),
        )
        .await;
    assert_eq!(answer.body().len(), payload.len());
    Ok(())
}

#[tokio::test]
#[ignore = "real-infra: end to end against a fake daemon on a real unix socket"]
async fn an_unregistered_remote_host_is_a_bad_gateway() -> TestResult {
    let (_dir, path) = socket()?;
    let transport = transport_at(path);
    let answer = transport
        .handle(
            &PaneOrigin::Core,
            request(Method::GET, "crowbar://host-7/v0/x", b""),
        )
        .await;
    assert_eq!(answer.status(), 502);
    Ok(())
}
