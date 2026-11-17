use std::io;
use std::sync::Arc;
use std::time::Duration;

use http_body_util::LengthLimitError;
use rustls::pki_types::ServerName;
use socket2::{SockRef, TcpKeepalive};
use tauri::http;
use tokio::net::TcpStream;
use tokio_rustls::client::TlsStream;
use tokio_rustls::TlsConnector;

use super::channel::proxy;
use super::proxy_answer::error_response;
use crate::host_error::HostError;
use crate::host_tokens::{Credential, TokenStore};
use crate::hosts::HostRegistry;
use crate::tls_pin::pinned_client_config;

mod dial_error;
mod remote_host;

pub(crate) use dial_error::DialError;
pub(crate) use remote_host::RemoteHost;

pub(crate) const REMOTE_BODY_LIMIT: usize = 256 * 1024 * 1024;

pub(crate) const DIAL_TIMEOUT: Duration = Duration::from_secs(5);

const SERVER_NAME: &str = "crowbar-remote";

const KEEPALIVE_IDLE: Duration = Duration::from_secs(15);
const KEEPALIVE_INTERVAL: Duration = Duration::from_secs(5);

pub(super) fn server_name() -> Result<ServerName<'static>, HostError> {
    ServerName::try_from(SERVER_NAME).map_err(|source| HostError::ServerName {
        name: SERVER_NAME,
        source,
    })
}

pub(crate) async fn dial(host: &RemoteHost) -> Result<TlsStream<TcpStream>, DialError> {
    tokio::time::timeout(DIAL_TIMEOUT, connect_pinned(host))
        .await
        .unwrap_or_else(|_| {
            let msg = format!("{} did not answer within {DIAL_TIMEOUT:?}", host.endpoint);
            Err(DialError::Unreachable(msg))
        })
}

async fn connect_pinned(host: &RemoteHost) -> Result<TlsStream<TcpStream>, DialError> {
    let tcp = host
        .endpoint
        .connect()
        .await
        .map_err(|e| DialError::Unreachable(e.to_string()))?;
    probe_when_idle(&tcp)
        .map_err(|e| DialError::Unreachable(format!("cannot reach {}: {e}", host.endpoint)))?;
    let config = pinned_client_config(host.pin.clone())
        .map_err(|e| DialError::Unreachable(format!("cannot reach {}: {e}", host.endpoint)))?;
    let connector = TlsConnector::from(config);
    connector
        .connect(
            server_name().map_err(|e| DialError::Unreachable(e.to_string()))?,
            tcp,
        )
        .await
        .map_err(|e| handshake_failure(host, &e))
}

fn probe_when_idle(tcp: &TcpStream) -> io::Result<()> {
    let keepalive = TcpKeepalive::new()
        .with_time(KEEPALIVE_IDLE)
        .with_interval(KEEPALIVE_INTERVAL);
    SockRef::from(tcp).set_tcp_keepalive(&keepalive)
}

fn handshake_failure(
    host: &RemoteHost,
    e: &io::Error,
) -> DialError {
    let rejected = e
        .get_ref()
        .and_then(|inner| inner.downcast_ref::<rustls::Error>())
        .is_some_and(|tls| matches!(tls, rustls::Error::InvalidCertificate(_)));
    if rejected {
        return DialError::IdentityChanged(format!(
            "{} does not hold the identity pinned as {} — this host's identity changed ({e})",
            host.endpoint, host.pin
        ));
    }
    DialError::Unreachable(format!("TLS handshake with {} failed: {e}", host.endpoint))
}

pub(super) async fn forward_remote(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
    mut request: http::Request<Vec<u8>>,
) -> http::Response<Vec<u8>> {
    let Some(endpoint) = registry.get(host_id) else {
        let msg = format!("unknown remote host {host_id}");
        return error_response(http::StatusCode::BAD_GATEWAY, &msg);
    };
    let credential = match stored_credential(tokens, host_id).await {
        Ok(Some(credential)) => credential,
        Ok(None) => return unpaired(),
        Err(e) => return lookup_failed(&e),
    };
    let authorization = match bearer(&credential.token) {
        Ok(authorization) => authorization,
        Err(e) => return error_response(http::StatusCode::INTERNAL_SERVER_ERROR, &e.to_string()),
    };
    request
        .headers_mut()
        .insert(http::header::AUTHORIZATION, authorization);
    relay(RemoteHost::paired(endpoint, &credential), request).await
}

fn unpaired() -> http::Response<Vec<u8>> {
    error_response(
        http::StatusCode::UNAUTHORIZED,
        "no stored credential for this host — pair it again",
    )
}

fn lookup_failed(error: &HostError) -> http::Response<Vec<u8>> {
    let msg = format!("token lookup failed: {error}");
    error_response(http::StatusCode::INTERNAL_SERVER_ERROR, &msg)
}

async fn relay(
    host: RemoteHost,
    request: http::Request<Vec<u8>>,
) -> http::Response<Vec<u8>> {
    let tls = match dial(&host).await {
        Ok(tls) => tls,
        Err(e) => return e.into_response(),
    };
    match proxy(tls, request, REMOTE_BODY_LIMIT).await {
        Ok(resp) => resp,
        Err(e) => relay_failure(&*e),
    }
}

fn relay_failure(e: &(dyn std::error::Error + Send + Sync + 'static)) -> http::Response<Vec<u8>> {
    if e.is::<LengthLimitError>() {
        return error_response(http::StatusCode::BAD_GATEWAY, "remote response too large");
    }
    error_response(
        http::StatusCode::BAD_GATEWAY,
        &format!("crowbar proxy error: {e}"),
    )
}

pub(crate) async fn stored_credential(
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
) -> Result<Option<Credential>, HostError> {
    let tokens = Arc::clone(tokens);
    let host_id = host_id.to_owned();
    let stored = tokio::task::spawn_blocking(move || tokens.get(&host_id))
        .await
        .map_err(HostError::Interrupted)??;
    Ok(stored.as_deref().and_then(Credential::decode))
}

pub(crate) fn bearer(token: &str) -> Result<http::HeaderValue, HostError> {
    let mut value = http::HeaderValue::from_str(&format!("Bearer {token}"))
        .map_err(HostError::InvalidCredential)?;
    value.set_sensitive(true);
    Ok(value)
}

#[cfg(test)]
mod tests {
    use std::time::Duration;

    use super::*;
    use crate::api_proxy::channel::collect_capped;
    use crate::api_proxy::proxy_answer::readable_by;
    use crate::api_proxy::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, PROXY_HEADER};
    use crate::hosts::endpoint::parse_endpoint;
    use crate::test_support::proxy_fakes::{
        header, identity, marker, paired, refused_endpoint, registry, response, serve_once_tls,
        unrelated_key, Keychain, HOST_ID,
    };
    use crate::tls_pin::{certificate_digest, Pin};

    fn request() -> http::Request<Vec<u8>> {
        http::Request::builder()
            .method(http::Method::GET)
            .uri("crowbar://host-1/v0/projects?limit=1")
            .body(Vec::new())
            .unwrap()
    }

    fn text(resp: &http::Response<Vec<u8>>) -> String {
        String::from_utf8_lossy(resp.body()).into_owned()
    }

    fn keychain_pinning(cert: &[u8]) -> Arc<dyn TokenStore> {
        Keychain::holding(&[(HOST_ID, &paired("sometoken", cert))])
    }

    async fn forward_as_paired_with(
        registry: &HostRegistry,
        cert: &[u8],
    ) -> http::Response<Vec<u8>> {
        forward_remote(registry, &keychain_pinning(cert), HOST_ID, request()).await
    }

    #[tokio::test]
    async fn a_request_reaches_the_pinned_host_with_its_stored_bearer_token() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let (addr, reported) =
            serve_once_tls(cert.clone(), key, Some(response("200 OK", "ok"))).await;
        let registry = registry(&addr.to_string());

        let resp = forward_as_paired_with(&registry, &cert).await;

        assert_eq!(resp.status(), 200, "{}", text(&resp));
        assert_eq!(text(&resp), "ok");
        let head = reported
            .await
            .unwrap()
            .expect("handshake with the pinned host");
        assert!(
            head.starts_with("GET /v0/projects?limit=1 HTTP/1.1\r\n"),
            "{head}"
        );
        assert_eq!(header(&head, "authorization"), Some("Bearer sometoken"));
    }

    #[tokio::test]
    async fn a_host_presenting_another_certificate_is_an_identity_change_before_any_request() {
        let _serialised = crate::test_support::fd_tests().await;
        let (pinned, _) = identity();
        let (impostor_cert, impostor_key) = identity();
        let (addr, reported) =
            serve_once_tls(impostor_cert, impostor_key, Some(response("200 OK", "ok"))).await;
        let registry = registry(&addr.to_string());

        let resp = forward_as_paired_with(&registry, &pinned).await;

        assert_eq!(resp.status(), 502, "{}", text(&resp));
        assert_eq!(marker(&resp), Some(HOST_IDENTITY_CHANGED));
        assert!(text(&resp).contains("identity changed"), "{}", text(&resp));
        let served = reported.await.unwrap();
        assert!(served.is_err(), "the impostor read a request: {served:?}");
        let page = http::HeaderValue::from_static("tauri://localhost");
        let resp = readable_by(Some(page.clone()), resp);
        let allowed = resp
            .headers()
            .get(http::header::ACCESS_CONTROL_ALLOW_ORIGIN);
        assert_eq!(allowed, Some(&page), "the page cannot read the marker");
    }

    #[tokio::test]
    async fn a_host_replaying_the_pinned_certificate_without_its_key_is_an_identity_change() {
        let _serialised = crate::test_support::fd_tests().await;
        let (pinned, _) = identity();
        let (addr, reported) = serve_once_tls(
            pinned.clone(),
            unrelated_key(),
            Some(response("200 OK", "ok")),
        )
        .await;
        let registry = registry(&addr.to_string());

        let resp = forward_as_paired_with(&registry, &pinned).await;

        assert_eq!(resp.status(), 502, "{}", text(&resp));
        assert_eq!(marker(&resp), Some(HOST_IDENTITY_CHANGED));
        let served = reported.await.unwrap();
        assert!(served.is_err(), "the replay read a request: {served:?}");
    }

    #[tokio::test]
    async fn an_unreachable_host_is_told_apart_from_an_identity_change() {
        let _serialised = crate::test_support::fd_tests().await;
        let registry = registry(&refused_endpoint());

        let resp = forward_as_paired_with(&registry, b"unused").await;

        assert_eq!(resp.status(), 503, "{}", text(&resp));
        assert_eq!(marker(&resp), Some(HOST_UNREACHABLE));
    }

    #[tokio::test(start_paused = true)]
    async fn a_host_that_never_completes_the_handshake_is_unreachable_at_the_dial_deadline() {
        let _serialised = crate::test_support::fd_tests().await;
        let silent = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let registry = registry(&silent.local_addr().unwrap().to_string());

        let answered = tokio::time::timeout(
            Duration::from_secs(60),
            forward_as_paired_with(&registry, b"unused"),
        );
        let resp = answered.await.expect("must fail at the dial deadline");

        assert_eq!(resp.status(), 503, "{}", text(&resp));
        assert_eq!(marker(&resp), Some(HOST_UNREACHABLE));
        assert!(
            text(&resp).contains("did not answer within"),
            "{}",
            text(&resp)
        );
    }

    #[tokio::test]
    async fn a_host_without_a_usable_credential_is_refused_before_dialing() {
        a_host_without_a_usable_credential_is_re_run().await;
    }

    async fn a_host_without_a_usable_credential_is_re_run() {
        let _serialised = crate::test_support::fd_tests().await;
        let registry = registry(&refused_endpoint());
        let cases = [
            (Keychain::holding(&[]), 401),
            (
                Keychain::holding(&[("another-host", &paired("t", b""))]),
                401,
            ),
            (Keychain::holding(&[(HOST_ID, &paired("", b""))]), 401),
            (Keychain::holding(&[(HOST_ID, &paired(" \t", b""))]), 401),
            (Keychain::holding(&[(HOST_ID, "sometoken")]), 401),
            (Keychain::broken("keychain is locked"), 500),
        ];
        for (case, (keychain, status)) in cases.into_iter().enumerate() {
            let answered = tokio::time::timeout(
                Duration::from_secs(5),
                forward_remote(&registry, &keychain, HOST_ID, request()),
            );
            let resp = answered.await.expect("must answer, not hang");
            assert_eq!(resp.status(), status, "case {case}: {}", text(&resp));
        }
    }

    #[tokio::test]
    async fn an_unregistered_host_is_not_proxied_even_while_reachable() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let (addr, _reported) =
            serve_once_tls(cert.clone(), key, Some(response("200 OK", "ok"))).await;
        let registry = registry(&addr.to_string());
        registry.remove(HOST_ID);

        let resp = forward_as_paired_with(&registry, &cert).await;

        assert_eq!(resp.status(), 502, "{}", text(&resp));
        assert!(
            text(&resp).contains("unknown remote host"),
            "{}",
            text(&resp)
        );
    }

    #[tokio::test]
    async fn a_host_cannot_answer_as_the_proxy() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let spoofed = format!(
            "HTTP/1.1 200 OK\r\n{PROXY_HEADER}: {HOST_IDENTITY_CHANGED}\r\ncontent-length: 2\r\nconnection: close\r\n\r\nok"
        );
        let (addr, _reported) = serve_once_tls(cert.clone(), key, Some(spoofed.into())).await;
        let registry = registry(&addr.to_string());

        let resp = forward_as_paired_with(&registry, &cert).await;

        assert_eq!(resp.status(), 200, "{}", text(&resp));
        assert_eq!(marker(&resp), None, "the host's marker was relayed");
    }

    #[tokio::test]
    async fn a_remote_connection_probes_its_peer_while_idle() {
        let _serialised = crate::test_support::fd_tests().await;
        let (cert, key) = identity();
        let (addr, _reported) = serve_once_tls(cert.clone(), key, None).await;
        let host = RemoteHost {
            endpoint: parse_endpoint(&addr.to_string()).unwrap(),
            pin: Pin::Paired(certificate_digest(&cert)),
        };

        let tls = dial(&host).await.map_err(|e| e.to_string()).unwrap();

        let socket = SockRef::from(tls.get_ref().0);
        assert!(socket.keepalive().unwrap(), "no keepalive probes");
        assert_eq!(socket.tcp_keepalive_time().unwrap(), KEEPALIVE_IDLE);
        assert_eq!(socket.tcp_keepalive_interval().unwrap(), KEEPALIVE_INTERVAL);
    }

    #[tokio::test]
    async fn an_oversized_answer_is_a_plain_502_without_the_proxy_marker() {
        let body = bytes::Bytes::from(vec![0u8; 8]);
        let error = collect_capped(http_body_util::Full::new(body), 4)
            .await
            .expect_err("a body past the limit");

        let resp = relay_failure(&*error);

        assert_eq!(resp.status(), 502);
        assert_eq!(text(&resp), "remote response too large");
        assert_eq!(marker(&resp), None);
    }
}
