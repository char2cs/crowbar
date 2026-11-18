use std::sync::Arc;

use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::crypto::{
    CryptoProvider, WebPkiSupportedAlgorithms, aws_lc_rs, verify_tls12_signature,
    verify_tls13_signature,
};
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{CertificateError, DigitallySignedStruct, Error as TlsError, SignatureScheme};

use crate::transport::remote::tls::config::client_config_with;
use crate::transport::remote::tls::pin::Pin;

#[derive(Debug)]
pub(crate) struct PinnedVerifier {
    pin: Pin,
    algorithms: WebPkiSupportedAlgorithms,
}

impl PinnedVerifier {
    pub(crate) fn new(
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
        if !self.pin.matches(end_entity.as_ref()) {
            return Err(TlsError::InvalidCertificate(
                CertificateError::ApplicationVerificationFailure,
            ));
        }
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

pub(crate) fn pinned_client_config(pin: Pin) -> Result<Arc<rustls::ClientConfig>, TlsError> {
    let provider = Arc::new(aws_lc_rs::default_provider());
    let verifier = Arc::new(PinnedVerifier::new(pin, &provider));
    client_config_with(provider, verifier)
}

#[cfg(test)]
mod tests {
    use std::sync::Arc;

    use super::*;
    use crate::transport::remote::tls::fingerprint::{certificate_digest, fingerprint_of};
    use rustls::client::danger::ServerCertVerifier;
    use rustls::crypto::aws_lc_rs;
    // SPEC: https://docs.rs/rustls/latest/rustls/struct.DigitallySignedStruct.html DigitallySignedStruct's constructor is pub(crate); decoding its wire form is the only way in.
    use rustls::internal::msgs::codec::Codec;
    use rustls::pki_types::{
        CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer, ServerName, UnixTime,
    };
    use rustls::sign::{CertifiedKey, SingleCertAndKey};
    use rustls::{
        CertificateError, DigitallySignedStruct, Error as TlsError, PeerIncompatible,
        SignatureScheme, SupportedProtocolVersion,
    };

    const TRANSCRIPT: &[u8] = b"handshake transcript the server proves key possession over";

    /// A real self-signed certificate (what the pinned daemon presents) and
    /// the private key it was issued for.
    fn generate_identity() -> (CertificateDer<'static>, PrivateKeyDer<'static>) {
        let certified = rcgen::generate_simple_self_signed(["localhost".to_owned()])
            .expect("generate self-signed test cert");
        let key = PrivatePkcs8KeyDer::from(certified.key_pair.serialize_der()).into();
        (certified.cert.der().clone(), key)
    }

    /// A fresh P-256 key unrelated to any pinned certificate.
    fn attacker_key() -> PrivateKeyDer<'static> {
        let key_pair = rcgen::KeyPair::generate().expect("generate attacker key");
        PrivatePkcs8KeyDer::from(key_pair.serialize_der()).into()
    }

    fn verifier_pinned_to(fingerprint: String) -> PinnedVerifier {
        verifier_pinning(Pin::Confirmed(fingerprint))
    }

    fn verifier_pinning(pin: Pin) -> PinnedVerifier {
        PinnedVerifier::new(pin, &aws_lc_rs::default_provider())
    }

    /// Signs `message` with `key` the way a server's `CertificateVerify` does.
    fn sign(
        key: &PrivateKeyDer<'_>,
        message: &[u8],
    ) -> DigitallySignedStruct {
        let scheme = SignatureScheme::ECDSA_NISTP256_SHA256;
        let signer = aws_lc_rs::sign::any_ecdsa_type(key)
            .expect("load P-256 key")
            .choose_scheme(&[scheme])
            .expect("a P-256 key signs ECDSA_NISTP256_SHA256");
        let signature = signer.sign(message).expect("sign message");
        let mut wire = scheme.to_array().to_vec();
        wire.extend_from_slice(&u16::try_from(signature.len()).unwrap().to_be_bytes());
        wire.extend_from_slice(&signature);
        DigitallySignedStruct::read_bytes(&wire).expect("decode DigitallySignedStruct")
    }

    /// Drives an in-memory handshake from `pinned_client_config` (pinned to
    /// `cert`) to a server that speaks `versions`, presents `cert` but signs
    /// with `key`.
    fn handshake(
        cert: &CertificateDer<'static>,
        key: PrivateKeyDer<'static>,
        versions: &[&'static SupportedProtocolVersion],
    ) -> Result<(), TlsError> {
        let provider = Arc::new(aws_lc_rs::default_provider());
        let signing_key = provider.key_provider.load_private_key(key)?;
        let resolver = SingleCertAndKey::from(CertifiedKey::new(vec![cert.clone()], signing_key));
        let server_config = rustls::ServerConfig::builder_with_provider(provider)
            .with_protocol_versions(versions)?
            .with_no_client_auth()
            .with_cert_resolver(Arc::new(resolver));
        let mut server = rustls::ServerConnection::new(Arc::new(server_config))?;
        let mut client = rustls::ClientConnection::new(
            pinned_client_config(Pin::Confirmed(fingerprint_of(cert)))?,
            ServerName::try_from("localhost").unwrap(),
        )?;
        while client.is_handshaking() || server.is_handshaking() {
            let mut to_server = Vec::new();
            client.write_tls(&mut to_server).unwrap();
            let mut to_client = Vec::new();
            server.write_tls(&mut to_client).unwrap();
            assert!(
                !(to_server.is_empty() && to_client.is_empty()),
                "handshake stalled"
            );
            deliver(&mut server, &to_server)?;
            deliver(&mut client, &to_client)?;
        }
        Ok(())
    }

    fn deliver<Side>(
        connection: &mut rustls::ConnectionCommon<Side>,
        mut inbound: &[u8],
    ) -> Result<(), TlsError> {
        while !inbound.is_empty() {
            connection.read_tls(&mut inbound).unwrap();
            connection.process_new_packets()?;
        }
        Ok(())
    }

    #[derive(Clone, Copy)]
    enum Signer {
        Pinned,
        Attacker,
    }

    fn bad_signature() -> TlsError {
        TlsError::InvalidCertificate(CertificateError::BadSignature)
    }

    /// (who signs, expected `verify_tls13_signature`/`verify_tls12_signature` result)
    const SIGNATURES: [(Signer, bool); 2] = [(Signer::Attacker, false), (Signer::Pinned, true)];

    #[test]
    fn handshake_signature_is_accepted_only_from_the_pinned_certs_key() {
        for (signer, accepted) in SIGNATURES {
            let (cert, key) = generate_identity();
            let verifier = verifier_pinned_to(fingerprint_of(&cert));
            let signed = match signer {
                Signer::Pinned => sign(&key, TRANSCRIPT),
                Signer::Attacker => sign(&attacker_key(), TRANSCRIPT),
            };
            let expected = accepted.then_some(()).ok_or_else(bad_signature);

            let tls13 = verifier.verify_tls13_signature(TRANSCRIPT, &cert, &signed);
            let tls12 = verifier.verify_tls12_signature(TRANSCRIPT, &cert, &signed);

            assert_eq!(tls13.map(|_| ()), expected, "TLS 1.3");
            assert_eq!(tls12.map(|_| ()), expected, "TLS 1.2");
        }
    }

    const TLS12_ONLY: &[&SupportedProtocolVersion] = &[&rustls::version::TLS12];

    /// (who signs the handshake, protocol versions the server speaks, expected outcome)
    type Handshake = (
        Signer,
        &'static [&'static SupportedProtocolVersion],
        Result<(), TlsError>,
    );

    fn handshake_cases() -> [Handshake; 3] {
        [
            (
                Signer::Attacker,
                rustls::DEFAULT_VERSIONS,
                Err(TlsError::InvalidCertificate(CertificateError::BadSignature)),
            ),
            (Signer::Pinned, rustls::DEFAULT_VERSIONS, Ok(())),
            (
                Signer::Pinned,
                TLS12_ONLY,
                Err(TlsError::PeerIncompatible(
                    PeerIncompatible::Tls12NotOfferedOrEnabled,
                )),
            ),
        ]
    }

    #[test]
    fn the_handshake_completes_only_for_the_pinned_key_over_tls13() {
        for (signer, versions, expected) in handshake_cases() {
            let (cert, key) = generate_identity();
            let key = match signer {
                Signer::Pinned => key,
                Signer::Attacker => attacker_key(),
            };
            assert_eq!(handshake(&cert, key, versions), expected);
        }
    }

    /// (does the verifier pin the presented certificate, expected `verify_server_cert` result)
    type Verdict = Result<(), TlsError>;

    const CERTIFICATES: [(bool, Verdict); 2] = [
        (true, Ok(())),
        (
            false,
            Err(TlsError::InvalidCertificate(
                CertificateError::ApplicationVerificationFailure,
            )),
        ),
    ];

    #[test]
    fn verify_server_cert_accepts_only_the_pinned_fingerprint() {
        for (pinned, expected) in CERTIFICATES {
            let (der, _) = generate_identity();
            let wrong_fingerprint = "SHA256:0000 0000 0000 0000 0000 0000".to_owned();
            assert_ne!(
                fingerprint_of(&der),
                wrong_fingerprint,
                "fixture must differ"
            );
            let candidates = [wrong_fingerprint, fingerprint_of(&der)];
            let fingerprint = candidates[usize::from(pinned)].clone();
            let verifier = verifier_pinned_to(fingerprint);
            let server_name = ServerName::try_from("localhost").unwrap();

            let result = verifier.verify_server_cert(&der, &[], &server_name, &[], UnixTime::now());

            assert_eq!(result.map(|_| ()), expected, "pinned: {pinned}");
        }
    }

    #[test]
    fn a_paired_pin_trusts_only_the_whole_digest_not_its_displayed_prefix() {
        let (der, _) = generate_identity();
        let digest = certificate_digest(&der);
        let mut same_prefix = digest;
        same_prefix[31] ^= 1;
        let server_name = ServerName::try_from("localhost").unwrap();
        let verify = |pin: Pin| {
            verifier_pinning(pin).verify_server_cert(&der, &[], &server_name, &[], UnixTime::now())
        };

        assert_eq!(
            Pin::Paired(same_prefix).to_string(),
            fingerprint_of(&der),
            "test fixture bug: the forged digest must show the same fingerprint"
        );
        assert!(verify(Pin::Paired(digest)).is_ok());
        assert_eq!(
            verify(Pin::Paired(same_prefix)).err(),
            Some(TlsError::InvalidCertificate(
                CertificateError::ApplicationVerificationFailure
            )),
            "a digest agreeing only in its first 96 bits must not verify"
        );
    }
}
