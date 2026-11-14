//! `crowbar://<hostId>/...`: a paired remote daemon, reached over TCP with TLS
//! pinned to the certificate pairing verified, authenticated by the bearer
//! token pairing stored beside that pin in the OS keychain.

use std::fmt;
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

use super::{error_response, proxy, proxy_error, HOST_IDENTITY_CHANGED, HOST_UNREACHABLE};
use crate::host_tokens::{Credential, TokenStore};
use crate::hosts::endpoint::Endpoint;
use crate::hosts::HostRegistry;
use crate::tls_pin::{pinned_client_config, Pin};

/// Most bytes the proxy buffers of one remote host's answer. The webview gets a
/// body whole, so a hostile host could otherwise stream until the desktop runs
/// out of memory; this is far above any honest answer.
pub(crate) const REMOTE_BODY_LIMIT: usize = 256 * 1024 * 1024;

/// Sent as SNI only: `PinnedVerifier` trusts the fingerprint and ignores names.
const SERVER_NAME: &str = "crowbar-remote";

/// Bound on connect plus handshake. A host that is up answers well within it;
/// one behind a dropped link would otherwise hold the request until the OS
/// abandons the connect.
pub(crate) const DIAL_TIMEOUT: Duration = Duration::from_secs(5);

/// A peer that dies silently (power loss, dropped VPN, expired NAT) never
/// closes the connection, and an idle TCP socket never notices on its own;
/// probing is how a stream to it ends rather than hangs.
const KEEPALIVE_IDLE: Duration = Duration::from_secs(15);
const KEEPALIVE_INTERVAL: Duration = Duration::from_secs(5);

/// Where a remote daemon listens, and the one certificate trusted there.
#[derive(Clone, Debug)]
pub(crate) struct RemoteHost {
    pub endpoint: Endpoint,
    pub pin: Pin,
}

impl RemoteHost {
    /// Pinned to the certificate `credential` was minted by — never to anything
    /// the registry, and so the webview, supplies.
    pub(crate) fn paired(endpoint: Endpoint, credential: &Credential) -> Self {
        Self {
            endpoint,
            pin: Pin::Paired(credential.pin),
        }
    }
}

pub(crate) enum DialError {
    /// Refused, timed out, or broke off mid-handshake.
    Unreachable(String),
    /// Answered, but failed pinned-certificate verification.
    IdentityChanged(String),
}

impl DialError {
    /// The `x-crowbar-proxy` value naming this failure, on every transport.
    pub(crate) fn marker(&self) -> &'static str {
        match self {
            Self::Unreachable(_) => HOST_UNREACHABLE,
            Self::IdentityChanged(_) => HOST_IDENTITY_CHANGED,
        }
    }

    fn into_response(self) -> http::Response<Vec<u8>> {
        let status = match self {
            Self::Unreachable(_) => 503,
            Self::IdentityChanged(_) => 502,
        };
        proxy_error(status, self.marker(), &self.to_string())
    }
}

impl fmt::Display for DialError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            Self::Unreachable(msg) | Self::IdentityChanged(msg) => f.write_str(msg),
        }
    }
}

/// TCP plus a TLS handshake that trusts only `host`'s pinned certificate.
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
        .map_err(DialError::Unreachable)?;
    probe_when_idle(&tcp)
        .map_err(|e| DialError::Unreachable(format!("cannot reach {}: {e}", host.endpoint)))?;
    let server_name = ServerName::try_from(SERVER_NAME).expect("SERVER_NAME is a DNS name");
    let connector = TlsConnector::from(pinned_client_config(host.pin.clone()));
    connector
        .connect(server_name, tcp)
        .await
        .map_err(|e| handshake_failure(host, &e))
}

fn probe_when_idle(tcp: &TcpStream) -> io::Result<()> {
    let keepalive = TcpKeepalive::new()
        .with_time(KEEPALIVE_IDLE)
        .with_interval(KEEPALIVE_INTERVAL);
    SockRef::from(tcp).set_tcp_keepalive(&keepalive)
}

/// A certificate the verifier rejected — another one, or the pinned one
/// replayed without its key — is an impostor, not an outage.
fn handshake_failure(host: &RemoteHost, e: &io::Error) -> DialError {
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

/// Relays `request` to paired host `host_id`. The credential is resolved before
/// dialing, so a host with none never receives an unauthenticated request.
pub(super) async fn forward_remote(
    registry: &HostRegistry,
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
    mut request: http::Request<Vec<u8>>,
) -> http::Response<Vec<u8>> {
    let Some(endpoint) = registry.get(host_id) else {
        return error_response(502, &format!("unknown remote host {host_id}"));
    };
    let credential = match stored_credential(tokens, host_id).await {
        Ok(Some(credential)) => credential,
        Ok(None) => {
            return error_response(401, "no stored credential for this host — pair it again")
        }
        Err(e) => return error_response(500, &format!("token lookup failed: {e}")),
    };
    let authorization = match bearer(&credential.token) {
        Ok(authorization) => authorization,
        Err(e) => return error_response(500, &e),
    };
    request
        .headers_mut()
        .insert(http::header::AUTHORIZATION, authorization);

    let tls = match dial(&RemoteHost::paired(endpoint, &credential)).await {
        Ok(tls) => tls,
        Err(e) => return e.into_response(),
    };
    match proxy(tls, request, REMOTE_BODY_LIMIT).await {
        Ok(resp) => resp,
        Err(e) => relay_failure(&*e),
    }
}

/// A relay that failed after the dial. An oversized answer gets a plain 502 of
/// its own: no proxy marker means "too large", and the host id stays out of it.
fn relay_failure(e: &(dyn std::error::Error + Send + Sync + 'static)) -> http::Response<Vec<u8>> {
    if e.is::<LengthLimitError>() {
        return error_response(502, "remote response too large");
    }
    error_response(502, &format!("crowbar proxy error: {e}"))
}

/// What pairing stored for `host_id`; any other keychain entry counts as none.
/// The keychain read can block on an OS prompt, so it runs on the blocking
/// pool rather than an async worker.
pub(crate) async fn stored_credential(
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
) -> Result<Option<Credential>, String> {
    let tokens = Arc::clone(tokens);
    let host_id = host_id.to_string();
    let stored = tokio::task::spawn_blocking(move || tokens.get(&host_id))
        .await
        .map_err(|e| e.to_string())??;
    Ok(stored.as_deref().and_then(Credential::decode))
}

/// `Bearer <token>`, marked sensitive so it never reaches a debug log.
pub(crate) fn bearer(token: &str) -> Result<http::HeaderValue, String> {
    let mut value = http::HeaderValue::from_str(&format!("Bearer {token}"))
        .map_err(|_| "the stored credential is not a valid header value".to_string())?;
    value.set_sensitive(true);
    Ok(value)
}

#[cfg(test)]
mod tests {
    use std::convert::Infallible;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use std::sync::Arc;
    use std::time::Duration;

    use bytes::Bytes;
    use futures_util::StreamExt;
    use http_body_util::{LengthLimitError, StreamBody};
    use hyper::body::Frame;
    use tauri::http;

    use super::super::collect_capped;
    use super::super::test_support::{
        header, identity, marker, paired, refused_endpoint, registry, response, serve_once_tls,
        unrelated_key, Keychain, HOST_ID,
    };
    use super::super::{readable_by, HOST_IDENTITY_CHANGED, HOST_UNREACHABLE, PROXY_HEADER};
    use super::{
        dial, forward_remote, relay_failure, RemoteHost, KEEPALIVE_IDLE, KEEPALIVE_INTERVAL,
    };
    use crate::host_tokens::TokenStore;
    use crate::hosts::endpoint::parse_endpoint;
    use crate::hosts::HostRegistry;
    use crate::tls_pin::{certificate_digest, Pin};

    fn request() -> http::Request<Vec<u8>> {
        http::Request::builder()
            .method(http::Method::GET)
            .uri("crowbar://host-1/v0/projects?limit=1")
            .body(Vec::new())
            .unwrap()
    }

    fn ok() -> Option<Vec<u8>> {
        Some(response("200 OK", "ok"))
    }

    fn text(resp: &http::Response<Vec<u8>>) -> String {
        String::from_utf8_lossy(resp.body()).into_owned()
    }

    fn keychain_pinning(cert: &[u8]) -> Arc<dyn TokenStore> {
        Keychain::holding(&[(HOST_ID, &paired("sometoken", cert))])
    }

    /// As a host paired with the server presenting `cert`.
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
        let (addr, reported) = serve_once_tls(cert.clone(), key, ok()).await;
        let registry = registry(addr.to_string());

        let resp = forward_as_paired_with(&registry, &cert).await;

        assert_eq!(resp.status(), 200, "{}", text(&resp));
        assert_eq!(text(&resp), "ok");
        let head = reported
            .await
            .expect("server reported")
            .expect("handshake with the pinned host");
        assert!(
            head.starts_with("GET /v0/projects?limit=1 HTTP/1.1\r\n"),
            "{head}"
        );
        assert_eq!(header(&head, "authorization"), Some("Bearer sometoken"));
    }

    // Regression: the pin came from register_remote_host, which the webview
    // calls, so re-registering a paired host could send its token elsewhere.
    #[tokio::test]
    async fn a_host_presenting_another_certificate_is_an_identity_change_before_any_request() {
        let _serialised = crate::test_support::fd_tests().await;
        let (pinned, _) = identity();
        let (impostor_cert, impostor_key) = identity();
        let (addr, reported) = serve_once_tls(impostor_cert, impostor_key, ok()).await;
        let registry = registry(addr.to_string());

        let resp = forward_as_paired_with(&registry, &pinned).await;

        assert_eq!(resp.status(), 502, "{}", text(&resp));
        assert_eq!(marker(&resp), Some(HOST_IDENTITY_CHANGED));
        assert!(text(&resp).contains("identity changed"), "{}", text(&resp));
        let served = reported.await.expect("server reported");
        assert!(
            served.is_err(),
            "the impostor completed a handshake and read: {served:?}"
        );
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
        let (addr, reported) = serve_once_tls(pinned.clone(), unrelated_key(), ok()).await;
        let registry = registry(addr.to_string());

        let resp = forward_as_paired_with(&registry, &pinned).await;

        assert_eq!(resp.status(), 502, "{}", text(&resp));
        assert_eq!(marker(&resp), Some(HOST_IDENTITY_CHANGED));
        let served = reported.await.expect("server reported");
        assert!(
            served.is_err(),
            "the replay completed a handshake and read: {served:?}"
        );
    }

    #[tokio::test]
    async fn an_unreachable_host_is_told_apart_from_an_identity_change() {
        let _serialised = crate::test_support::fd_tests().await;
        let registry = registry(refused_endpoint());

        let resp = forward_as_paired_with(&registry, b"unused").await;

        assert_eq!(resp.status(), 503, "{}", text(&resp));
        assert_eq!(marker(&resp), Some(HOST_UNREACHABLE));
    }

    #[tokio::test(start_paused = true)]
    async fn a_host_that_never_completes_the_handshake_is_unreachable_at_the_dial_deadline() {
        let _serialised = crate::test_support::fd_tests().await;
        // Listening, so the kernel completes the TCP handshake, but never speaks TLS.
        let silent = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let registry = registry(silent.local_addr().unwrap().to_string());

        let answered = tokio::time::timeout(
            Duration::from_secs(60),
            forward_as_paired_with(&registry, b"unused"),
        );
        let resp = answered
            .await
            .expect("must fail at the dial deadline, not hang");

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
        let _serialised = crate::test_support::fd_tests().await;
        // A dial would reach the refused endpoint and answer 503, not these.
        let registry = registry(refused_endpoint());
        let cases = [
            (Keychain::holding(&[]), 401),
            (
                Keychain::holding(&[("another-host", &paired("t", b""))]),
                401,
            ),
            (Keychain::holding(&[(HOST_ID, &paired("", b""))]), 401),
            (Keychain::holding(&[(HOST_ID, &paired(" \t", b""))]), 401),
            // Stored before tokens carried their pin: pairing again binds one.
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
        let (addr, _reported) = serve_once_tls(cert.clone(), key, ok()).await;
        let registry = registry(addr.to_string());
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
        let registry = registry(addr.to_string());

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

        let socket = socket2::SockRef::from(tls.get_ref().0);
        assert!(socket.keepalive().unwrap(), "no keepalive probes");
        assert_eq!(socket.tcp_keepalive_time().unwrap(), KEEPALIVE_IDLE);
        assert_eq!(socket.tcp_keepalive_interval().unwrap(), KEEPALIVE_INTERVAL);
    }

    /// A body of `chunks` 1 KiB frames that counts how many were pulled.
    fn counted_body(
        chunks: usize,
        pulled: Arc<AtomicUsize>,
    ) -> impl hyper::body::Body<Data = Bytes, Error = Infallible> {
        StreamBody::new(futures_util::stream::iter(0..chunks).map(move |_| {
            pulled.fetch_add(1, Ordering::SeqCst);
            Ok(Frame::data(Bytes::from_static(&[0u8; 1024])))
        }))
    }

    #[tokio::test]
    async fn an_oversized_remote_answer_is_cut_off_while_it_streams() {
        let pulled = Arc::new(AtomicUsize::new(0));

        let err = collect_capped(counted_body(1024, pulled.clone()), 4 * 1024)
            .await
            .expect_err("a body past the limit must not be buffered");

        assert!(err.is::<LengthLimitError>(), "{err}");
        assert!(
            pulled.load(Ordering::SeqCst) <= 5,
            "read {} KiB of a 4 KiB limit",
            pulled.load(Ordering::SeqCst)
        );
        let resp = relay_failure(&*err);
        assert_eq!(resp.status(), 502);
        assert_eq!(text(&resp), "remote response too large");
        assert_eq!(marker(&resp), None);
    }

    #[tokio::test]
    async fn an_answer_within_the_limit_is_relayed_whole() {
        let body = collect_capped(counted_body(3, Arc::default()), 4 * 1024)
            .await
            .expect("within the limit");

        assert_eq!(body.len(), 3 * 1024);
    }
}
