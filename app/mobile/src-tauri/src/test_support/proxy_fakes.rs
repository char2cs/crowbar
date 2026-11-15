//! Stand-in remote daemons and a keychain for the proxy tests.

use std::collections::HashMap;
use std::net::SocketAddr;
use std::sync::{Arc, Mutex};

use rustls::crypto::aws_lc_rs;
use rustls::pki_types::{CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer};
use rustls::sign::{CertifiedKey, SingleCertAndKey};
use rustls::SupportedProtocolVersion;
use tauri::http;
use tokio::io::{AsyncRead, AsyncReadExt, AsyncWriteExt, DuplexStream};
use tokio::net::TcpListener;
use tokio::sync::{mpsc, oneshot};
use tokio_rustls::TlsAcceptor;

use crate::api_proxy::PROXY_HEADER;
use crate::host_tokens::{Credential, TokenStore};
use crate::hosts::endpoint::parse_endpoint;
use crate::hosts::HostRegistry;
use crate::tls_pin::certificate_digest;

pub(crate) const HOST_ID: &str = "host-1";

const DUPLEX_BUFFER: usize = 64 * 1024;

pub(crate) fn identity() -> (CertificateDer<'static>, PrivateKeyDer<'static>) {
    let certified = rcgen::generate_simple_self_signed(["localhost".to_owned()])
        .expect("generate test identity");
    let key = PrivatePkcs8KeyDer::from(certified.key_pair.serialize_der()).into();
    (certified.cert.der().clone(), key)
}

pub(crate) fn unrelated_key() -> PrivateKeyDer<'static> {
    let key_pair = rcgen::KeyPair::generate().expect("generate key");
    PrivatePkcs8KeyDer::from(key_pair.serialize_der()).into()
}

pub(crate) struct Keychain {
    tokens: Mutex<HashMap<String, String>>,
    broken: Option<&'static str>,
}

impl Keychain {
    pub(crate) fn holding(entries: &[(&str, &str)]) -> Arc<dyn TokenStore> {
        let tokens = entries
            .iter()
            .map(|(host, token)| (host.to_string(), token.to_string()))
            .collect();
        Arc::new(Self {
            tokens: Mutex::new(tokens),
            broken: None,
        })
    }

    pub(crate) fn broken(reason: &'static str) -> Arc<dyn TokenStore> {
        Arc::new(Self {
            tokens: Mutex::default(),
            broken: Some(reason),
        })
    }

    fn check(&self) -> Result<(), String> {
        self.broken.map_or(Ok(()), |reason| Err(reason.to_owned()))
    }
}

impl TokenStore for Keychain {
    fn get(
        &self,
        host_id: &str,
    ) -> Result<Option<String>, String> {
        self.check()?;
        Ok(self.tokens.lock().unwrap().get(host_id).cloned())
    }

    fn set(
        &self,
        host_id: &str,
        token: &str,
    ) -> Result<(), String> {
        self.check()?;
        self.tokens
            .lock()
            .unwrap()
            .insert(host_id.to_owned(), token.to_owned());
        Ok(())
    }

    fn delete(
        &self,
        _: &str,
    ) -> Result<(), String> {
        unreachable!("the remote path never deletes a credential")
    }
}

pub(crate) fn paired(
    token: &str,
    cert: &[u8],
) -> String {
    Credential {
        token: token.to_owned(),
        pin: certificate_digest(cert),
    }
    .encode()
}

pub(crate) fn registry(endpoint: &str) -> HostRegistry {
    let registry = HostRegistry::default();
    let endpoint = parse_endpoint(endpoint).unwrap();
    registry.insert(HOST_ID.into(), endpoint).unwrap();
    registry
}

pub(crate) fn refused_endpoint() -> String {
    let listener = std::net::TcpListener::bind("127.0.0.1:0").unwrap();
    listener.local_addr().unwrap().to_string()
}

pub(crate) fn response(
    status: &str,
    body: &str,
) -> Vec<u8> {
    format!(
        "HTTP/1.1 {status}\r\ncontent-type: application/json\r\ncontent-length: {}\r\nconnection: close\r\n\r\n{body}",
        body.len()
    )
    .into_bytes()
}

pub(crate) async fn serve_once_tls(
    cert: CertificateDer<'static>,
    key: PrivateKeyDer<'static>,
    reply: Option<Vec<u8>>,
) -> (SocketAddr, oneshot::Receiver<Result<String, String>>) {
    serve_once_tls_speaking(rustls::DEFAULT_VERSIONS, cert, key, reply).await
}

pub(crate) async fn serve_once_tls_speaking(
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
    tokio::spawn(serve_one(listener, acceptor, report, reply));
    (addr, reported)
}

async fn serve_one(
    listener: TcpListener,
    acceptor: TlsAcceptor,
    report: oneshot::Sender<Result<String, String>>,
    reply: Option<Vec<u8>>,
) {
    let (tcp, _) = listener.accept().await.expect("accept");
    let mut tls = match acceptor.accept(tcp).await {
        Ok(tls) => tls,
        Err(e) => return drop(report.send(Err(e.to_string()))),
    };
    drop(report.send(Ok(read_request(&mut tls).await)));
    match reply {
        Some(bytes) => tls.write_all(&bytes).await.expect("write response"),
        None => std::future::pending().await,
    }
}

pub(crate) fn kept_alive(
    status: &str,
    body: &str,
) -> Vec<u8> {
    format!(
        "HTTP/1.1 {status}\r\ncontent-type: application/json\r\ncontent-length: {}\r\n\r\n{body}",
        body.len()
    )
    .into_bytes()
}

pub(crate) async fn serve_tls_exchanges(
    cert: CertificateDer<'static>,
    key: PrivateKeyDer<'static>,
    replies: Vec<Vec<u8>>,
) -> (SocketAddr, mpsc::UnboundedReceiver<String>) {
    let listener = TcpListener::bind("127.0.0.1:0")
        .await
        .expect("bind test remote daemon");
    let addr = listener.local_addr().unwrap();
    let acceptor = tls_acceptor(rustls::DEFAULT_VERSIONS, cert, key);
    let (report, reported) = mpsc::unbounded_channel();
    tokio::spawn(exchange(listener, acceptor, replies, report));
    (addr, reported)
}

async fn exchange(
    listener: TcpListener,
    acceptor: TlsAcceptor,
    replies: Vec<Vec<u8>>,
    report: mpsc::UnboundedSender<String>,
) {
    let (tcp, _) = listener.accept().await.expect("accept");
    let mut tls = acceptor.accept(tcp).await.expect("handshake");
    for reply in replies {
        drop(report.send(read_request(&mut tls).await));
        tls.write_all(&reply).await.expect("write response");
    }
}

pub(crate) fn duplex_peer(reply: Vec<u8>) -> (DuplexStream, oneshot::Receiver<String>) {
    let (client, mut server) = tokio::io::duplex(DUPLEX_BUFFER);
    let (report, reported) = oneshot::channel();
    tokio::spawn(async move {
        drop(report.send(read_request(&mut server).await));
        server.write_all(&reply).await.expect("write response");
    });
    (client, reported)
}

pub(crate) fn tls_acceptor(
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

fn is_complete(
    text: &str,
    received: usize,
) -> bool {
    let Some(end) = text.find("\r\n\r\n") else {
        return false;
    };
    let length = header(text, "content-length").map_or(0, |v| v.parse().unwrap());
    received >= end + 4 + length
}

async fn fill(
    stream: &mut (impl AsyncRead + Unpin),
    buf: &mut Vec<u8>,
) -> bool {
    let mut chunk = [0u8; 4096];
    let n = stream.read(&mut chunk).await.expect("read request");
    buf.extend_from_slice(&chunk[..n]);
    n > 0
}

fn head_is_complete(buf: &[u8]) -> bool {
    is_complete(&String::from_utf8_lossy(buf), buf.len())
}

pub(crate) async fn read_request(stream: &mut (impl AsyncRead + Unpin)) -> String {
    let mut buf = Vec::new();
    while !head_is_complete(&buf) && fill(stream, &mut buf).await {}
    String::from_utf8_lossy(&buf).into_owned()
}

pub(crate) fn header<'a>(
    head: &'a str,
    name: &str,
) -> Option<&'a str> {
    head.split("\r\n\r\n")
        .next()?
        .lines()
        .skip(1)
        .find_map(|line| {
            let (key, value) = line.split_once(':')?;
            key.eq_ignore_ascii_case(name).then_some(value.trim())
        })
}

pub(crate) fn marker(resp: &http::Response<Vec<u8>>) -> Option<&str> {
    resp.headers()
        .get(PROXY_HEADER)
        .map(|v| v.to_str().unwrap())
}
