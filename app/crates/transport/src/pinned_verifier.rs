use std::sync::Arc;

use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::crypto::{
    CryptoProvider, WebPkiSupportedAlgorithms, aws_lc_rs, verify_tls12_signature,
    verify_tls13_signature,
};
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{CertificateError, DigitallySignedStruct, Error as TlsError, SignatureScheme};

use crate::pin::Pin;
use crate::tls_config::client_config_with;

#[derive(Debug)]
pub(crate) struct PinnedVerifier {
    pin: Pin,
    algorithms: WebPkiSupportedAlgorithms,
}

impl PinnedVerifier {
    pub(crate) fn new(pin: Pin, provider: &CryptoProvider) -> Self {
        Self {
            pin,
            algorithms: provider.signature_verification_algorithms,
        }
    }
}

impl ServerCertVerifier for PinnedVerifier {
    fn verify_server_cert(
        &self,
        end_entity: &CertificateDer<'_>,
        _intermediates: &[CertificateDer<'_>],
        _server_name: &ServerName<'_>,
        _ocsp_response: &[u8],
        _now: UnixTime,
    ) -> Result<ServerCertVerified, TlsError> {
        if self.pin.matches(end_entity.as_ref()) {
            Ok(ServerCertVerified::assertion())
        } else {
            Err(TlsError::InvalidCertificate(
                CertificateError::ApplicationVerificationFailure,
            ))
        }
    }

    fn verify_tls12_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, TlsError> {
        verify_tls12_signature(message, cert, dss, &self.algorithms)
    }

    fn verify_tls13_signature(
        &self,
        message: &[u8],
        cert: &CertificateDer<'_>,
        dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, TlsError> {
        verify_tls13_signature(message, cert, dss, &self.algorithms)
    }

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        vec![SignatureScheme::ECDSA_NISTP256_SHA256]
    }
}

/// A rustls client config trusting only `pin`'s certificate. The crypto provider is explicit so
/// signature checks never depend on a process-wide default; TLS 1.3 only.
///
/// # Errors
/// Fails when the crypto provider offers no TLS 1.3 suite.
pub fn pinned_client_config(pin: Pin) -> Result<Arc<rustls::ClientConfig>, TlsError> {
    let provider = Arc::new(aws_lc_rs::default_provider());
    let verifier = Arc::new(PinnedVerifier::new(pin, &provider));
    client_config_with(provider, verifier)
}

#[cfg(test)]
#[path = "pinned_verifier_tests.rs"]
mod tests;
