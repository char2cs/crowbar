use std::net::SocketAddr;
use std::sync::{Arc, Mutex};

use rustls::pki_types::{CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer};
use tokio::net::{TcpListener, TcpStream};
use tokio_rustls::TlsAcceptor;

use crate::http_daemon::{Handler, Log, serve_http};

/// A fake remote daemon: TLS with a self-signed certificate it owns, speaking HTTP/1.1.
pub(crate) struct TlsHost {
    pub(crate) addr: SocketAddr,
    pub(crate) cert_der: Vec<u8>,
    pub(crate) log: Log,
}

pub(crate) struct Identity {
    pub(crate) cert: CertificateDer<'static>,
    pub(crate) key: PrivateKeyDer<'static>,
}

pub(crate) fn identity() -> Identity {
    let certified = rcgen::generate_simple_self_signed(["localhost".to_owned()])
        .expect("self-signed certificate");
    Identity {
        cert: certified.cert.der().clone(),
        key: PrivateKeyDer::Pkcs8(PrivatePkcs8KeyDer::from(certified.key_pair.serialize_der())),
    }
}

impl TlsHost {
    pub(crate) async fn start(
        identity: Identity,
        versions: &[&'static rustls::SupportedProtocolVersion],
        handler: Handler,
    ) -> Self {
        let cert_der = identity.cert.to_vec();
        let config = rustls::ServerConfig::builder_with_protocol_versions(versions)
            .with_no_client_auth()
            .with_single_cert(vec![identity.cert], identity.key)
            .expect("server config");
        let acceptor = TlsAcceptor::from(Arc::new(config));
        let listener = TcpListener::bind(("127.0.0.1", 0))
            .await
            .expect("bind tls host");
        let addr = listener.local_addr().expect("tls host address");
        let log: Log = Arc::new(Mutex::new(Vec::new()));
        tokio::spawn(accept(listener, acceptor, handler, Arc::clone(&log)));
        Self {
            addr,
            cert_der,
            log,
        }
    }

    pub(crate) fn endpoint(&self) -> String {
        self.addr.to_string()
    }

    pub(crate) fn requests(&self) -> Vec<crate::http_daemon::Seen> {
        self.log.lock().expect("log lock").clone()
    }
}

async fn serve_tls(acceptor: TlsAcceptor, tcp: TcpStream, handler: Handler, log: Log) {
    if let Ok(tls) = acceptor.accept(tcp).await {
        serve_http(tls, handler, log).await;
    }
}

async fn accept(listener: TcpListener, acceptor: TlsAcceptor, handler: Handler, log: Log) {
    loop {
        let Ok((tcp, _)) = listener.accept().await else {
            return;
        };
        let (acceptor, handler, log) = (acceptor.clone(), Arc::clone(&handler), Arc::clone(&log));
        tokio::spawn(serve_tls(acceptor, tcp, handler, log));
    }
}
