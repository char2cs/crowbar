//! A stand-in remote daemon and keychain for the remote-path tests.

use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::{Arc, Mutex};

use rustls::crypto::aws_lc_rs;
use rustls::pki_types::{CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer};
use rustls::sign::{CertifiedKey, SingleCertAndKey};
use rustls::SupportedProtocolVersion;
use tauri::http;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWriteExt};
use tokio::net::TcpListener;
use tokio::sync::oneshot;
use tokio_rustls::TlsAcceptor;

use super::PROXY_HEADER;
use crate::host_tokens::TokenStore;
use crate::hosts::{HostRegistry, RemoteHost};

pub const HOST_ID: &str = "host-1";

/// A self-signed P-256 identity, the kind api/internal/core/tlsid generates.
pub fn identity() -> (CertificateDer<'static>, PrivateKeyDer<'static>) {
    let certified = rcgen::generate_simple_self_signed(["localhost".to_string()])
        .expect("generate test identity");
    let key = PrivatePkcs8KeyDer::from(certified.key_pair.serialize_der()).into();
    (certified.cert.der().clone(), key)
}

/// A fresh P-256 key no certificate was issued for.
pub fn unrelated_key() -> PrivateKeyDer<'static> {
    let key_pair = rcgen::KeyPair::generate().expect("generate key");
    PrivatePkcs8KeyDer::from(key_pair.serialize_der()).into()
}

/// An in-memory keychain; a broken one fails every call.
pub struct Keychain {
    tokens: Mutex<HashMap<String, String>>,
    broken: Option<&'static str>,
}

impl Keychain {
    pub fn holding(entries: &[(&str, &str)]) -> Arc<dyn TokenStore> {
        let tokens = entries
            .iter()
            .map(|(host, token)| (host.to_string(), token.to_string()))
            .collect();
        Arc::new(Self {
            tokens: Mutex::new(tokens),
            broken: None,
        })
    }

    pub fn broken(reason: &'static str) -> Arc<dyn TokenStore> {
        Arc::new(Self {
            tokens: Mutex::default(),
            broken: Some(reason),
        })
    }

    fn check(&self) -> Result<(), String> {
        self.broken.map_or(Ok(()), |reason| Err(reason.to_string()))
    }
}

impl TokenStore for Keychain {
    fn get(&self, host_id: &str) -> Result<Option<String>, String> {
        self.check()?;
        Ok(self.tokens.lock().unwrap().get(host_id).cloned())
    }
    fn set(&self, host_id: &str, token: &str) -> Result<(), String> {
        self.check()?;
        self.tokens
            .lock()
            .unwrap()
            .insert(host_id.to_string(), token.to_string());
        Ok(())
    }
    fn delete(&self, _: &str) -> Result<(), String> {
        unreachable!("the remote path never deletes a credential")
    }
}

pub fn registry(endpoint: String, fingerprint: String) -> HostRegistry {
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

/// An endpoint nothing listens on.
pub fn refused_endpoint() -> String {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    listener.local_addr().unwrap().to_string()
}

pub fn response(status: &str, body: &str) -> Vec<u8> {
    format!(
        "HTTP/1.1 {status}\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
        body.len()
    )
    .into_bytes()
}

/// Terminates TLS as [`tls_acceptor`] does. Reports the request it read, or
/// why the handshake failed, then sends `reply` or, with none, holds the
/// connection open without answering.
pub async fn serve_once_tls(
    cert: CertificateDer<'static>,
    key: PrivateKeyDer<'static>,
    reply: Option<Vec<u8>>,
) -> (SocketAddr, oneshot::Receiver<Result<String, String>>) {
    serve_once_tls_speaking(rustls::DEFAULT_VERSIONS, cert, key, reply).await
}

/// `serve_once_tls`, offering only the protocol `versions`.
pub async fn serve_once_tls_speaking(
    versions: &[&'static SupportedProtocolVersion],
    cert: CertificateDer<'static>,
    key: PrivateKeyDer<'static>,
    reply: Option<Vec<u8>>,
) -> (SocketAddr, oneshot::Receiver<Result<String, String>>) {
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind test remote daemon");
    let addr = listener.local_addr().unwrap();
    let acceptor = tls_acceptor(versions, cert, key);
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
        let _ = report.send(Ok(read_request(&mut tls).await));
        match reply {
            Some(bytes) => tls.write_all(&bytes).await.expect("write response"),
            None => std::future::pending().await,
        }
    });
    (addr, reported)
}

/// Presents `cert` and signs with `key` — which need not match, so a test can
/// replay a certificate it holds no key for.
pub fn tls_acceptor(
    versions: &[&'static SupportedProtocolVersion],
    cert: CertificateDer<'static>,
    key: PrivateKeyDer<'static>,
) -> TlsAcceptor {
    let provider = Arc::new(aws_lc_rs::default_provider());
    let signing_key = provider
        .key_provider
        .load_private_key(key)
        .expect("load key");
    let resolver = SingleCertAndKey::from(CertifiedKey::new(vec![cert], signing_key));
    let config = rustls::ServerConfig::builder_with_provider(provider)
        .with_protocol_versions(versions)
        .expect("protocol versions")
        .with_no_client_auth()
        .with_cert_resolver(Arc::new(resolver));
    TlsAcceptor::from(Arc::new(config))
}

async fn read_request(stream: &mut (impl AsyncRead + Unpin)) -> String {
    let mut buf = Vec::new();
    let mut chunk = [0u8; 4096];
    loop {
        let text = String::from_utf8_lossy(&buf).into_owned();
        if let Some(end) = text.find("\r\n\r\n") {
            let length = header(&text, "content-length").map_or(0, |v| v.parse().unwrap());
            if buf.len() >= end + 4 + length {
                return text;
            }
        }
        let n = stream.read(&mut chunk).await.expect("read request");
        if n == 0 {
            return text;
        }
        buf.extend_from_slice(&chunk[..n]);
    }
}

/// Header `name`'s value in a raw request head; hyper writes names lowercase.
pub fn header<'a>(head: &'a str, name: &str) -> Option<&'a str> {
    head.split("\r\n\r\n")
        .next()?
        .lines()
        .skip(1)
        .find_map(|line| {
            let (key, value) = line.split_once(':')?;
            key.eq_ignore_ascii_case(name).then_some(value.trim())
        })
}

pub fn marker(resp: &http::Response<Vec<u8>>) -> Option<&str> {
    resp.headers()
        .get(PROXY_HEADER)
        .map(|v| v.to_str().unwrap())
}
