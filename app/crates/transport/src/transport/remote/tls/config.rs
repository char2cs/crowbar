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
mod tests {
    use rustls::crypto::aws_lc_rs;

    use super::*;
    use crate::transport::remote::tls::pin::Pin;
    use crate::transport::remote::tls::pinned_verifier::PinnedVerifier;

    #[test]
    fn a_config_is_built_for_tls13_with_a_custom_verifier() {
        let provider = Arc::new(aws_lc_rs::default_provider());
        let verifier = Arc::new(PinnedVerifier::new(Pin::Paired([0; 32]), &provider));
        assert!(client_config_with(provider, verifier).is_ok());
    }
}
