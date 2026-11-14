use std::io;
use std::time::Duration;

use rustls::pki_types::ServerName;
use socket2::{SockRef, TcpKeepalive};
use tokio::net::TcpStream;
use tokio_rustls::TlsConnector;
use tokio_rustls::client::TlsStream;

use crate::error::Error;
use crate::pinned_verifier::pinned_client_config;
use crate::remote_host::RemoteHost;

pub(crate) const SERVER_NAME: &str = "crowbar-remote";

pub(crate) const DIAL_TIMEOUT: Duration = Duration::from_secs(5);

const KEEPALIVE_IDLE: Duration = Duration::from_secs(15);

const KEEPALIVE_INTERVAL: Duration = Duration::from_secs(5);

pub(crate) async fn dial(host: &RemoteHost) -> Result<TlsStream<TcpStream>, Error> {
    match tokio::time::timeout(DIAL_TIMEOUT, connect_pinned(host)).await {
        Ok(result) => result,
        Err(_) => Err(Error::HostUnreachable(format!(
            "{} did not answer within {DIAL_TIMEOUT:?}",
            host.endpoint
        ))),
    }
}

async fn connect_pinned(host: &RemoteHost) -> Result<TlsStream<TcpStream>, Error> {
    let unreachable = |why: String| Error::HostUnreachable(why);
    let tcp = host
        .endpoint
        .connect()
        .await
        .map_err(|e| unreachable(e.to_string()))?;
    enable_keepalive(&tcp)
        .map_err(|e| unreachable(format!("cannot reach {}: {e}", host.endpoint)))?;
    let name = ServerName::try_from(SERVER_NAME).map_err(|e| unreachable(e.to_string()))?;
    let config = pinned_client_config(host.pin.clone()).map_err(|e| unreachable(e.to_string()))?;
    TlsConnector::from(config)
        .connect(name, tcp)
        .await
        .map_err(|e| handshake_failure(host, &e))
}

fn enable_keepalive(tcp: &TcpStream) -> io::Result<()> {
    let keepalive = TcpKeepalive::new()
        .with_time(KEEPALIVE_IDLE)
        .with_interval(KEEPALIVE_INTERVAL);
    SockRef::from(tcp).set_tcp_keepalive(&keepalive)
}

fn handshake_failure(host: &RemoteHost, e: &io::Error) -> Error {
    let rejected = e
        .get_ref()
        .and_then(|inner| inner.downcast_ref::<rustls::Error>())
        .is_some_and(|tls| matches!(tls, rustls::Error::InvalidCertificate(_)));
    if rejected {
        return Error::IdentityChanged(format!(
            "{} does not hold the identity pinned as {} \u{2014} this host's identity changed ({e})",
            host.endpoint, host.pin
        ));
    }
    Error::HostUnreachable(format!("TLS handshake with {} failed: {e}", host.endpoint))
}

#[cfg(test)]
#[path = "dial_tests.rs"]
mod tests;
