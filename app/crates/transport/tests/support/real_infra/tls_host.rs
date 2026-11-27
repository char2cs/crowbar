use std::net::SocketAddr;
use std::sync::{Arc, Mutex};

use rustls::pki_types::{CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer};
use tokio::net::{TcpListener, TcpStream};
use tokio_rustls::TlsAcceptor;

use crate::http_daemon::{Handler, Log, serve_http};

pub(crate) struct TlsHost {
    pub(crate) addr: SocketAddr,
    pub(crate) cert_der: Vec<u8>,
    pub(crate) log: Log,
}

pub(crate) struct Identity {
    pub(crate) cert: CertificateDer<'static>,
    pub(crate) key: PrivateKeyDer<'static>,
}

pub(crate) fn identity() -> Result<Identity, rcgen::Error> {
    let certified = rcgen::generate_simple_self_signed(["localhost".to_owned()])?;
    Ok(Identity {
        cert: certified.cert.der().clone(),
        key: PrivateKeyDer::Pkcs8(PrivatePkcs8KeyDer::from(certified.key_pair.serialize_der())),
    })
}

impl TlsHost {
    pub(crate) async fn start(
        identity: Identity,
        versions: &[&'static rustls::SupportedProtocolVersion],
        handler: Handler,
    ) -> Result<Self, Box<dyn std::error::Error>> {
        let cert_der = identity.cert.to_vec();
        let provider = Arc::new(rustls::crypto::aws_lc_rs::default_provider());
        let config = rustls::ServerConfig::builder_with_provider(provider)
            .with_protocol_versions(versions)?
            .with_no_client_auth()
            .with_single_cert(vec![identity.cert], identity.key)?;
        let acceptor = TlsAcceptor::from(Arc::new(config));
        let listener = TcpListener::bind(("127.0.0.1", 0)).await?;
        let addr = listener.local_addr()?;
        let log: Log = Arc::new(Mutex::new(Vec::new()));
        tokio::spawn(accept(listener, acceptor, handler, Arc::clone(&log)));
        Ok(Self {
            addr,
            cert_der,
            log,
        })
    }

    pub(crate) fn endpoint(&self) -> String {
        self.addr.to_string()
    }

    pub(crate) fn requests(&self) -> Vec<crate::http_daemon::Seen> {
        self.log
            .lock()
            .unwrap_or_else(std::sync::PoisonError::into_inner)
            .clone()
    }
}

async fn serve_tls(
    acceptor: TlsAcceptor,
    tcp: TcpStream,
    handler: Handler,
    log: Log,
) {
    if let Ok(tls) = acceptor.accept(tcp).await {
        serve_http(tls, handler, log).await;
    }
}

async fn accept(
    listener: TcpListener,
    acceptor: TlsAcceptor,
    handler: Handler,
    log: Log,
) {
    while let Ok((tcp, _)) = listener.accept().await {
        let (acceptor, handler, log) = (acceptor.clone(), Arc::clone(&handler), Arc::clone(&log));
        tokio::spawn(serve_tls(acceptor, tcp, handler, log));
    }
}
