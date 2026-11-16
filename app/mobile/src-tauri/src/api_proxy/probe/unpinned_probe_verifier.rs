use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::crypto::{verify_tls12_signature, verify_tls13_signature, WebPkiSupportedAlgorithms};
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{DigitallySignedStruct, Error as TlsError, SignatureScheme};

// SAFETY: trusts ANY certificate, a MITM hole anywhere but first contact. Sound here only because
// SAFETY: nothing is sent and a human compares the result with `crowbar pair`; signatures are
// SAFETY: still checked, so the host must hold the key of the certificate it presents.
#[derive(Debug)]
pub(super) struct UnpinnedProbeVerifier {
    pub(super) algorithms: WebPkiSupportedAlgorithms,
}

impl ServerCertVerifier for UnpinnedProbeVerifier {
    fn verify_server_cert(
        &self,
        _end_entity: &CertificateDer<'_>,
        _intermediates: &[CertificateDer<'_>],
        _server_name: &ServerName<'_>,
        _ocsp_response: &[u8],
        _now: UnixTime,
    ) -> Result<ServerCertVerified, TlsError> {
        Ok(ServerCertVerified::assertion())
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

    // PLATFORM: rustls accepts only what the pinned client accepts, so any host probed is one pairing can reach.
    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        vec![SignatureScheme::ECDSA_NISTP256_SHA256]
    }
}

#[cfg(test)]
mod tests {
    use rustls::crypto::aws_lc_rs;

    use super::*;

    fn verifier() -> UnpinnedProbeVerifier {
        UnpinnedProbeVerifier {
            algorithms: aws_lc_rs::default_provider().signature_verification_algorithms,
        }
    }

    #[test]
    fn any_certificate_is_accepted_but_only_the_pinned_clients_scheme_is_offered() {
        let cert = CertificateDer::from(vec![1, 2, 3]);
        let name = ServerName::try_from("anything").unwrap();

        let verdict = verifier().verify_server_cert(&cert, &[], &name, &[], UnixTime::now());

        assert!(verdict.is_ok(), "{verdict:?}");
        assert_eq!(
            verifier().supported_verify_schemes(),
            vec![SignatureScheme::ECDSA_NISTP256_SHA256]
        );
    }
}
