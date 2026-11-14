use std::sync::Arc;
use std::time::Duration;

use rustls::crypto::aws_lc_rs;
use rustls::pki_types::ServerName;
use tokio::io::AsyncWriteExt;
use tokio_rustls::TlsConnector;

use crate::dial::SERVER_NAME;
use crate::endpoint::Endpoint;
use crate::error::Error;
use crate::fingerprint::fingerprint_of;
use crate::tls_config::client_config_with;
use crate::unpinned_probe_verifier::UnpinnedProbeVerifier;

/// Bound on connect, handshake and hang-up; the user is waiting on a dialog.
pub const PROBE_TIMEOUT: Duration = Duration::from_secs(5);

/// The fingerprint of the certificate `endpoint` presents, for the user to confirm before pairing
/// pins it. This is the one unpinned TLS dial: it sends nothing and hangs up.
///
/// # Errors
/// [`Error::MalformedEndpoint`], [`Error::ProbeTimeout`], [`Error::Handshake`] or
/// [`Error::NoCertificate`].
pub async fn probe_fingerprint(endpoint: &str) -> Result<String, Error> {
    let endpoint = Endpoint::parse(endpoint)?;
    match tokio::time::timeout(PROBE_TIMEOUT, read_fingerprint(&endpoint)).await {
        Ok(result) => result,
        Err(_) => Err(Error::ProbeTimeout(endpoint)),
    }
}

async fn read_fingerprint(endpoint: &Endpoint) -> Result<String, Error> {
    let handshake = |reason: String| Error::Handshake {
        endpoint: endpoint.clone(),
        reason,
    };
    let tcp = endpoint.connect().await?;
    let name = ServerName::try_from(SERVER_NAME).map_err(|e| handshake(e.to_string()))?;
    let provider = Arc::new(aws_lc_rs::default_provider());
    let algorithms = provider.signature_verification_algorithms;
    let verifier = Arc::new(UnpinnedProbeVerifier { algorithms });
    let config = client_config_with(provider, verifier).map_err(|e| handshake(e.to_string()))?;
    let mut tls = TlsConnector::from(config)
        .connect(name, tcp)
        .await
        .map_err(|e| handshake(e.to_string()))?;
    let fingerprint = tls
        .get_ref()
        .1
        .peer_certificates()
        .and_then(|chain| chain.first())
        .map(|cert| fingerprint_of(cert))
        .ok_or_else(|| Error::NoCertificate(endpoint.clone()))?;
    tls.shutdown().await.ok();
    Ok(fingerprint)
}

#[cfg(test)]
#[path = "probe_tests.rs"]
mod tests;
