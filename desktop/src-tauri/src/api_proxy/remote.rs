//! `crowbar://<hostId>/...`: a paired remote daemon, reached over TCP with TLS
//! pinned to the fingerprint confirmed at pairing, authenticated by the bearer
//! token pairing stored in the OS keychain.

use std::sync::Arc;

use rustls::pki_types::ServerName;
use tauri::http;
use tokio::net::TcpStream;
use tokio_rustls::TlsConnector;

use super::{error_response, proxy};
use crate::host_tokens::TokenStore;
use crate::hosts::HostRegistry;
use crate::tls_pin::pinned_client_config;

/// Sent as SNI only: `PinnedVerifier` trusts the fingerprint and ignores names.
const SERVER_NAME: &str = "crowbar-remote";

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
    let Ok(mut authorization) = http::HeaderValue::from_str(&format!("Bearer {token}")) else {
        return error_response(500, "the stored credential is not a valid header value");
    };
    authorization.set_sensitive(true);
    request
        .headers_mut()
        .insert(http::header::AUTHORIZATION, authorization);

    let tcp = match TcpStream::connect(&host.endpoint).await {
        Ok(tcp) => tcp,
        Err(e) => return error_response(502, &format!("cannot reach {}: {e}", host.endpoint)),
    };
    let server_name = ServerName::try_from(SERVER_NAME).expect("SERVER_NAME is a DNS name");
    let connector = TlsConnector::from(pinned_client_config(host.fingerprint));
    let tls = match connector.connect(server_name, tcp).await {
        Ok(tls) => tls,
        Err(e) => {
            let msg = format!("TLS handshake with {} failed: {e}", host.endpoint);
            return error_response(502, &msg);
        }
    };
    match proxy(tls, request).await {
        Ok(resp) => resp,
        Err(e) => error_response(502, &format!("crowbar proxy error: {e}")),
    }
}

/// A blank credential counts as none. The keychain read can block on an OS
/// prompt, so it runs on the blocking pool rather than an async worker.
async fn stored_token(
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

#[cfg(test)]
mod tests {
    use std::net::SocketAddr;
    use std::sync::Arc;
    use std::time::Duration;

    use rustls::pki_types::{CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer};
    use tauri::http;
    use tokio::io::{AsyncRead, AsyncReadExt, AsyncWriteExt};
    use tokio::net::TcpListener;
    use tokio::sync::oneshot;
    use tokio_rustls::TlsAcceptor;

    use super::forward_remote;
    use crate::host_tokens::TokenStore;
    use crate::hosts::{HostRegistry, RemoteHost};
    use crate::tls_pin::fingerprint_of;

    const HOST_ID: &str = "host-1";

    fn request() -> http::Request<Vec<u8>> {
        http::Request::builder()
            .method(http::Method::GET)
            .uri("crowbar://host-1/v0/projects?limit=1")
            .body(Vec::new())
            .unwrap()
    }

    /// A self-signed P-256 identity, the kind api/internal/core/tlsid generates.
    fn identity() -> (CertificateDer<'static>, PrivateKeyDer<'static>) {
        let certified = rcgen::generate_simple_self_signed(["localhost".to_string()])
            .expect("generate test identity");
        let key = PrivatePkcs8KeyDer::from(certified.key_pair.serialize_der()).into();
        (certified.cert.der().clone(), key)
    }

    struct Tokens(Result<Option<&'static str>, &'static str>);

    impl TokenStore for Tokens {
        fn get(&self, _: &str) -> Result<Option<String>, String> {
            self.0
                .map(|t| t.map(str::to_string))
                .map_err(str::to_string)
        }
        fn set(&self, _: &str, _: &str) -> Result<(), String> {
            unreachable!("the proxy never writes a credential")
        }
        fn delete(&self, _: &str) -> Result<(), String> {
            unreachable!("the proxy never deletes a credential")
        }
    }

    fn tokens(stored: Result<Option<&'static str>, &'static str>) -> Arc<dyn TokenStore> {
        Arc::new(Tokens(stored))
    }

    async fn forward_as_paired(registry: &HostRegistry) -> http::Response<Vec<u8>> {
        forward_remote(registry, &tokens(Ok(Some("sometoken"))), HOST_ID, request()).await
    }

    fn registry(endpoint: String, fingerprint: String) -> HostRegistry {
        let registry = HostRegistry::default();
        registry.insert(
            HOST_ID.into(),
            RemoteHost {
                endpoint,
                fingerprint,
            },
        );
        registry
    }

    /// An endpoint nothing listens on: a dial there fails fast with a 502.
    fn refused_endpoint() -> String {
        let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
        listener.local_addr().unwrap().to_string()
    }

    /// Terminates TLS as `cert` for one connection and reports the request
    /// head it read — or why the handshake failed — then answers `200 ok`.
    async fn serve_once_tls(
        cert: CertificateDer<'static>,
        key: PrivateKeyDer<'static>,
    ) -> (SocketAddr, oneshot::Receiver<Result<String, String>>) {
        let listener = TcpListener::bind("127.0.0.1:0")
            .await
            .expect("bind test remote daemon");
        let addr = listener.local_addr().unwrap();
        let config = rustls::ServerConfig::builder()
            .with_no_client_auth()
            .with_single_cert(vec![cert], key)
            .expect("test server config");
        let acceptor = TlsAcceptor::from(Arc::new(config));
        let (report, reported) = oneshot::channel();
        tokio::spawn(async move {
            let (tcp, _) = listener.accept().await.expect("accept");
            let mut tls = match acceptor.accept(tcp).await {
                Ok(tls) => tls,
                Err(e) => {
                    let _ = report.send(Err(e.to_string()));
                    return;
                }
            };
            let _ = report.send(Ok(read_head(&mut tls).await));
            tls.write_all(b"HTTP/1.1 200 OK\r\ncontent-length: 2\r\nconnection: close\r\n\r\nok")
                .await
                .expect("write response");
        });
        (addr, reported)
    }

    async fn read_head(stream: &mut (impl AsyncRead + Unpin)) -> String {
        let mut buf = vec![0u8; 4096];
        let mut read = 0;
        loop {
            let n = stream.read(&mut buf[read..]).await.expect("read request");
            read += n;
            if n == 0 || buf[..read].windows(4).any(|w| w == b"\r\n\r\n") {
                break;
            }
        }
        String::from_utf8_lossy(&buf[..read]).into_owned()
    }

    /// Header `name`'s value in a raw request head; hyper writes names lowercase.
    fn header<'a>(head: &'a str, name: &str) -> Option<&'a str> {
        head.lines().skip(1).find_map(|line| {
            let (key, value) = line.split_once(':')?;
            key.eq_ignore_ascii_case(name).then_some(value.trim())
        })
    }

    #[tokio::test]
    async fn a_request_reaches_the_pinned_host_with_its_stored_bearer_token() {
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, reported) = serve_once_tls(cert, key).await;
        let registry = registry(addr.to_string(), fingerprint);

        let resp = forward_as_paired(&registry).await;

        let body = String::from_utf8_lossy(resp.body()).into_owned();
        assert_eq!(resp.status(), 200, "{body}");
        assert_eq!(body, "ok");
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
    async fn a_host_presenting_another_certificate_fails_the_handshake_before_any_request() {
        let (pinned, _) = identity();
        let (impostor_cert, impostor_key) = identity();
        let (addr, reported) = serve_once_tls(impostor_cert, impostor_key).await;
        let registry = registry(addr.to_string(), fingerprint_of(&pinned));

        let resp = forward_as_paired(&registry).await;

        let body = String::from_utf8_lossy(resp.body()).into_owned();
        assert_eq!(resp.status(), 502, "{body}");
        assert!(
            body.contains("TLS handshake") && body.contains("does not match the pinned one"),
            "expected the pinned verifier's rejection, got: {body}"
        );
        let served = reported.await.expect("server reported");
        assert!(
            served.is_err(),
            "the impostor completed a handshake and read: {served:?}"
        );
    }

    #[tokio::test]
    async fn a_host_without_a_usable_credential_is_refused_before_dialing() {
        // A dial would reach the refused endpoint and answer 502, not these.
        let registry = registry(refused_endpoint(), "SHA256:unused".into());
        let cases = [
            (Ok(None), 401),
            (Ok(Some("")), 401),
            (Ok(Some(" \t")), 401),
            (Err("keychain is locked"), 500),
        ];
        for (stored, status) in cases {
            let store = tokens(stored);
            let answered = tokio::time::timeout(
                Duration::from_secs(5),
                forward_remote(&registry, &store, HOST_ID, request()),
            );
            let resp = answered.await.expect("must answer, not hang");
            assert_eq!(
                resp.status(),
                status,
                "stored credential {stored:?}: {}",
                String::from_utf8_lossy(resp.body())
            );
        }
    }

    #[tokio::test]
    async fn an_unregistered_host_is_not_proxied_even_while_reachable() {
        let (cert, key) = identity();
        let fingerprint = fingerprint_of(&cert);
        let (addr, _reported) = serve_once_tls(cert, key).await;
        let registry = registry(addr.to_string(), fingerprint);
        registry.remove(HOST_ID);

        let resp = forward_as_paired(&registry).await;

        let body = String::from_utf8_lossy(resp.body()).into_owned();
        assert_eq!(resp.status(), 502, "{body}");
        assert!(body.contains("unknown remote host"), "{body}");
    }
}
