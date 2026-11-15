use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::crypto::{WebPkiSupportedAlgorithms, verify_tls12_signature, verify_tls13_signature};
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{DigitallySignedStruct, Error as TlsError, SignatureScheme};

#[derive(Debug)]
pub(crate) struct UnpinnedProbeVerifier {
    pub(crate) algorithms: WebPkiSupportedAlgorithms,
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

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        vec![SignatureScheme::ECDSA_NISTP256_SHA256]
    }
}

#[cfg(test)]
mod tests {
    use rustls::crypto::aws_lc_rs;
    // PLATFORM: DigitallySignedStruct's constructor is pub(crate); decoding its wire form is the only way in.
    use rustls::internal::msgs::codec::Codec;

    use super::*;

    fn verifier() -> UnpinnedProbeVerifier {
        UnpinnedProbeVerifier {
            algorithms: aws_lc_rs::default_provider().signature_verification_algorithms,
        }
    }

    fn accepts_any_certificate(verifier: &UnpinnedProbeVerifier) -> bool {
        let certified =
            rcgen::generate_simple_self_signed(["anywhere.example".to_owned()]).unwrap();
        let cert = certified.cert.der().clone();
        let name = ServerName::try_from("localhost").unwrap();
        verifier
            .verify_server_cert(&cert, &[], &name, &[], UnixTime::now())
            .is_ok()
    }

    fn accepts_a_forged_signature(verifier: &UnpinnedProbeVerifier) -> bool {
        let certified = rcgen::generate_simple_self_signed(["localhost".to_owned()]).unwrap();
        let cert = certified.cert.der().clone();
        let mut wire = SignatureScheme::ECDSA_NISTP256_SHA256.to_array().to_vec();
        wire.extend_from_slice(&64u16.to_be_bytes());
        wire.extend_from_slice(&[0u8; 64]);
        let forged = DigitallySignedStruct::read_bytes(&wire).unwrap();
        verifier
            .verify_tls13_signature(b"message", &cert, &forged)
            .is_ok()
    }

    type Check = fn(&UnpinnedProbeVerifier) -> bool;

    /// (what is presented, whether the probe verifier lets it through): the probe sends nothing,
    /// so any certificate passes, but the host must still hold the key it signs with.
    const CASES: [(&str, Check, bool); 2] = [
        ("an arbitrary certificate", accepts_any_certificate, true),
        (
            "a forged handshake signature",
            accepts_a_forged_signature,
            false,
        ),
    ];

    #[test]
    fn the_certificate_is_not_checked_but_the_handshake_signature_still_is() {
        for (presented, accepts, expected) in CASES {
            assert_eq!(accepts(&verifier()), expected, "{presented}");
        }
    }

    #[test]
    fn it_offers_only_the_signature_scheme_it_can_check() {
        assert_eq!(
            verifier().supported_verify_schemes(),
            vec![SignatureScheme::ECDSA_NISTP256_SHA256]
        );
    }
}
