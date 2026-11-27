use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::crypto::{
    verify_tls12_signature, verify_tls13_signature, CryptoProvider, WebPkiSupportedAlgorithms,
};
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{CertificateError, DigitallySignedStruct, Error as TlsError, SignatureScheme};

use super::pin::Pin;

#[derive(Debug)]
pub(super) struct PinnedVerifier {
    pin: Pin,
    algorithms: WebPkiSupportedAlgorithms,
}

impl PinnedVerifier {
    pub(super) fn new(
        pin: Pin,
        provider: &CryptoProvider,
    ) -> Self {
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
            return Ok(ServerCertVerified::assertion());
        }
        Err(TlsError::InvalidCertificate(
            CertificateError::ApplicationVerificationFailure,
        ))
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

#[cfg(test)]
mod tests {
    use super::*;
    use crate::tls_pin::digest::{certificate_digest, fingerprint_of};
    use rustls::crypto::aws_lc_rs;

    const DER: &[u8] = b"certificate bytes";

    fn verify(pin: Pin) -> Result<ServerCertVerified, TlsError> {
        let verifier = PinnedVerifier::new(pin, &aws_lc_rs::default_provider());
        let der = CertificateDer::from(DER);
        let name = ServerName::try_from("localhost").unwrap();
        verifier.verify_server_cert(&der, &[], &name, &[], UnixTime::now())
    }

    #[test]
    fn only_the_pinned_certificate_verifies() {
        let mut same_prefix = certificate_digest(DER);
        same_prefix[31] ^= 1;
        let refused = Err(TlsError::InvalidCertificate(
            CertificateError::ApplicationVerificationFailure,
        ));
        let cases = [
            (Pin::Confirmed(fingerprint_of(DER)), Ok(())),
            (
                Pin::Confirmed("SHA256:0000 0000 0000 0000 0000 0000".into()),
                refused.clone(),
            ),
            (Pin::Paired(certificate_digest(DER)), Ok(())),
            (Pin::Paired(same_prefix), refused),
        ];
        for (pin, expected) in cases {
            let label = pin.to_string();
            assert_eq!(verify(pin).map(drop), expected, "{label}");
        }
    }

    #[test]
    fn only_the_daemons_ecdsa_scheme_is_offered() {
        let verifier = PinnedVerifier::new(
            Pin::Paired(certificate_digest(DER)),
            &aws_lc_rs::default_provider(),
        );
        assert_eq!(
            verifier.supported_verify_schemes(),
            vec![SignatureScheme::ECDSA_NISTP256_SHA256]
        );
    }
}
