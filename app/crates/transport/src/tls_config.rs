use std::sync::Arc;

use rustls::client::danger::ServerCertVerifier;
use rustls::crypto::CryptoProvider;

pub(crate) fn client_config_with(
    provider: Arc<CryptoProvider>,
    verifier: Arc<dyn ServerCertVerifier>,
) -> Result<Arc<rustls::ClientConfig>, rustls::Error> {
    let config = rustls::ClientConfig::builder_with_provider(provider)
        .with_protocol_versions(&[&rustls::version::TLS13])?
        .dangerous()
        .with_custom_certificate_verifier(verifier)
        .with_no_client_auth();
    Ok(Arc::new(config))
}

#[cfg(test)]
#[path = "tls_config_tests.rs"]
mod tests;
