//! `crowbar://<hostId>/...`: a paired remote daemon, reached over TCP with TLS
//! pinned to the fingerprint confirmed at pairing, authenticated by the bearer
//! token pairing stored in the OS keychain.

use std::fmt;
use std::io;
use std::sync::Arc;
use std::time::Duration;

use rustls::pki_types::ServerName;
use tauri::http;
use tokio::net::TcpStream;
use tokio_rustls::client::TlsStream;
use tokio_rustls::TlsConnector;

use super::{error_response, proxy, proxy_error, HOST_IDENTITY_CHANGED, HOST_UNREACHABLE};
use crate::host_tokens::TokenStore;
use crate::hosts::{HostRegistry, RemoteHost};
use crate::tls_pin::pinned_client_config;

/// Sent as SNI only: `PinnedVerifier` trusts the fingerprint and ignores names.
const SERVER_NAME: &str = "crowbar-remote";

/// Bound on connect plus handshake. A host that is up answers well within it;
/// one behind a dropped link would otherwise hold the request until the OS
/// abandons the connect.
pub(crate) const DIAL_TIMEOUT: Duration = Duration::from_secs(5);

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
    let tcp = TcpStream::connect(&host.endpoint)
        .await
        .map_err(|e| DialError::Unreachable(format!("cannot reach {}: {e}", host.endpoint)))?;
    let server_name = ServerName::try_from(SERVER_NAME).expect("SERVER_NAME is a DNS name");
    let connector = TlsConnector::from(pinned_client_config(host.fingerprint.clone()));
    connector
        .connect(server_name, tcp)
        .await
        .map_err(|e| handshake_failure(host, &e))
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
            host.endpoint, host.fingerprint
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
    let Some(host) = registry.get(host_id) else {
        return error_response(502, &format!("unknown remote host {host_id}"));
    };
    let token = match stored_token(tokens, host_id).await {
        Ok(Some(token)) => token,
        Ok(None) => {
            return error_response(401, "no stored credential for this host — pair it again")
        }
        Err(e) => return error_response(500, &format!("token lookup failed: {e}")),
    };
    let authorization = match bearer(&token) {
        Ok(authorization) => authorization,
        Err(e) => return error_response(500, &e),
    };
    request
        .headers_mut()
        .insert(http::header::AUTHORIZATION, authorization);

    let tls = match dial(&host).await {
        Ok(tls) => tls,
        Err(e) => return e.into_response(),
    };
    match proxy(tls, request).await {
        Ok(resp) => resp,
        Err(e) => error_response(502, &format!("crowbar proxy error: {e}")),
    }
}

/// A blank credential counts as none. The keychain read can block on an OS
/// prompt, so it runs on the blocking pool rather than an async worker.
pub(crate) async fn stored_token(
    tokens: &Arc<dyn TokenStore>,
    host_id: &str,
) -> Result<Option<String>, String> {
    let tokens = Arc::clone(tokens);
    let host_id = host_id.to_string();
    let stored = tokio::task::spawn_blocking(move || tokens.get(&host_id))
        .await
        .map_err(|e| e.to_string())??;
    Ok(stored.filter(|token| !token.trim().is_empty()))
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
    use std::time::Duration;

    use tauri::http;

    use super::super::test_support::{
        header, identity, marker, refused_endpoint, registry, response, serve_once_tls,
        unrelated_key, Keychain, HOST_ID,
    };
    use super::super::{HOST_IDENTITY_CHANGED, HOST_UNREACHABLE};
    use super::forward_remote;
    use crate::hosts::HostRegistry;
    use crate::tls_pin::fingerprint_of;

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

    async fn forward_as_paired(registry: &HostRegistry) -> http::Response<Vec<u8>> {
        let keychain = Keychain::holding(&[(HOST_ID, "sometoken")]);
        forward_remote(registry, &keychain, HOST_ID, request()).await
    }

    #[tokio::test]
    async fn a_request_reaches_the_pinned_host_with_its_stored_bearer_token() {
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, reported) = serve_once_tls(cert, key, ok()).await;
        let registry = registry(addr.to_string(), fingerprint);

        let resp = forward_as_paired(&registry).await;

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

    #[tokio::test]
    async fn a_host_presenting_another_certificate_is_an_identity_change_before_any_request() {
        let (pinned, _) = identity();
        let (impostor_cert, impostor_key) = identity();
        let (addr, reported) = serve_once_tls(impostor_cert, impostor_key, ok()).await;
        let registry = registry(addr.to_string(), fingerprint_of(&pinned));

        let resp = forward_as_paired(&registry).await;

        assert_eq!(resp.status(), 502, "{}", text(&resp));
        assert_eq!(marker(&resp), Some(HOST_IDENTITY_CHANGED));
        assert!(text(&resp).contains("identity changed"), "{}", text(&resp));
        let served = reported.await.expect("server reported");
        assert!(
            served.is_err(),
            "the impostor completed a handshake and read: {served:?}"
        );
    }

    #[tokio::test]
    async fn a_host_replaying_the_pinned_certificate_without_its_key_is_an_identity_change() {
        let (pinned, _) = identity();
        let (addr, reported) = serve_once_tls(pinned.clone(), unrelated_key(), ok()).await;
        let registry = registry(addr.to_string(), fingerprint_of(&pinned));

        let resp = forward_as_paired(&registry).await;

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
        let registry = registry(refused_endpoint(), "SHA256:unused".into());

        let resp = forward_as_paired(&registry).await;

        assert_eq!(resp.status(), 503, "{}", text(&resp));
        assert_eq!(marker(&resp), Some(HOST_UNREACHABLE));
    }

    #[tokio::test(start_paused = true)]
    async fn a_host_that_never_completes_the_handshake_is_unreachable_at_the_dial_deadline() {
        // Listening, so the kernel completes the TCP handshake, but never speaks TLS.
        let silent = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        let registry = registry(silent.local_addr().unwrap().to_string(), "SHA256:x".into());

        let answered = tokio::time::timeout(Duration::from_secs(60), forward_as_paired(&registry));
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
        // A dial would reach the refused endpoint and answer 503, not these.
        let registry = registry(refused_endpoint(), "SHA256:unused".into());
        let cases = [
            (Keychain::holding(&[]), 401),
            (Keychain::holding(&[("another-host", "sometoken")]), 401),
            (Keychain::holding(&[(HOST_ID, "")]), 401),
            (Keychain::holding(&[(HOST_ID, " \t")]), 401),
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
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, _reported) = serve_once_tls(cert, key, ok()).await;
        let registry = registry(addr.to_string(), fingerprint);
        registry.remove(HOST_ID);

        let resp = forward_as_paired(&registry).await;

        assert_eq!(resp.status(), 502, "{}", text(&resp));
        assert!(
            text(&resp).contains("unknown remote host"),
            "{}",
            text(&resp)
        );
    }
}
