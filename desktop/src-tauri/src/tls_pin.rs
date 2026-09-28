//! Fingerprint pinning for remote daemon connections: instead of validating a
//! certificate chain against a CA (there is none — every daemon's identity is
//! self-signed, `api/internal/core/tlsid`), a remote host's certificate is
//! trusted exactly when its fingerprint matches the one the user confirmed
//! during pairing.

use std::sync::Arc;

use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
use rustls::crypto::{
    aws_lc_rs, verify_tls12_signature, verify_tls13_signature, CryptoProvider,
    WebPkiSupportedAlgorithms,
};
use rustls::pki_types::{CertificateDer, ServerName, UnixTime};
use rustls::{DigitallySignedStruct, Error as TlsError, SignatureScheme};
use sha2::{Digest, Sha256};

/// Renders a certificate's SHA-256 as grouped lowercase hex — byte-identical
/// in format to the Go daemon's `tlsid.FingerprintOf`, so the string a human
/// reads from `crowbar pair` on the daemon's own host and the string shown
/// here during pairing are directly comparable.
pub fn fingerprint_of(der: &[u8]) -> String {
    let hash = Sha256::digest(der);
    let full: String = hash.iter().map(|b| format!("{b:02x}")).collect();
    let groups: Vec<&str> = (0..6).map(|i| &full[i * 4..(i + 1) * 4]).collect();
    format!("SHA256:{}", groups.join(" "))
}

/// Whether `der`'s fingerprint matches `expected` (as produced by
/// `fingerprint_of` or by the Go daemon's `tlsid.FingerprintOf`).
pub fn fingerprint_matches(der: &[u8], expected: &str) -> bool {
    fingerprint_of(der) == expected
}

/// Trusts exactly one certificate: the one whose fingerprint was confirmed
/// during pairing. No CA chain, no hostname check — trust-on-first-connect,
/// pinned thereafter.
#[derive(Debug)]
struct PinnedVerifier {
    fingerprint: String,
    algorithms: WebPkiSupportedAlgorithms,
}

impl PinnedVerifier {
    fn new(fingerprint: String, provider: &CryptoProvider) -> Self {
        Self {
            fingerprint,
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
        if fingerprint_matches(end_entity.as_ref(), &self.fingerprint) {
            Ok(ServerCertVerified::assertion())
        } else {
            Err(TlsError::General(format!(
                "remote host's certificate fingerprint does not match the pinned one \
                 (expected {}) — this host's identity changed",
                self.fingerprint
            )))
        }
    }

    // A certificate's bytes are public and replayable; only this signature
    // proves the peer holds the pinned certificate's private key.
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
        // The daemon's identity is ECDSA P-256 (api/internal/core/tlsid.go
        // generates elliptic.P256 keys) — TLS 1.3 signs with ECDSA-SHA256.
        vec![SignatureScheme::ECDSA_NISTP256_SHA256]
    }
}

/// Builds a rustls client config that trusts only the certificate matching
/// `fingerprint`. The crypto provider is explicit so signature checks never
/// depend on whichever process-wide default happens to be installed. Both
/// ends are ours, so only TLS 1.3 is offered.
#[allow(dead_code)] // dead_code: no caller until the TLS-dial task in this plan wires this into the real transport
pub fn pinned_client_config(fingerprint: String) -> Arc<rustls::ClientConfig> {
    let provider = Arc::new(aws_lc_rs::default_provider());
    let verifier = Arc::new(PinnedVerifier::new(fingerprint, &provider));
    Arc::new(
        rustls::ClientConfig::builder_with_provider(provider)
            .with_protocol_versions(&[&rustls::version::TLS13])
            .expect("aws-lc-rs's default provider has TLS 1.3 cipher suites")
            .dangerous()
            .with_custom_certificate_verifier(verifier)
            .with_no_client_auth(),
    )
}

#[cfg(test)]
mod tests {
    use super::fingerprint_of;

    #[test]
    fn fingerprint_of_is_deterministic() {
        let der = b"some fake certificate bytes for a unit test";
        assert_eq!(fingerprint_of(der), fingerprint_of(der));
    }

    #[test]
    fn fingerprint_of_has_the_go_side_format() {
        // Format contract with api/internal/core/tlsid/tlsid.go's FingerprintOf:
        // "SHA256:" + six groups of 4 lowercase hex chars, space-separated —
        // the first 24 hex chars (12 bytes) of the SHA-256 digest, truncated
        // the same way on both sides so a human can visually compare them.
        let fp = fingerprint_of(b"anything");
        assert!(fp.starts_with("SHA256:"));
        let groups: Vec<&str> = fp["SHA256:".len()..].trim().split(' ').collect();
        assert_eq!(groups.len(), 6, "expected 6 groups, got {:?}", groups);
        for g in groups {
            assert_eq!(
                g.len(),
                4,
                "expected each group to be 4 hex chars, got {:?}",
                g
            );
            assert!(g.chars().all(|c| c.is_ascii_hexdigit()));
        }
    }

    #[test]
    fn fingerprint_of_distinguishes_different_input() {
        assert_ne!(fingerprint_of(b"a"), fingerprint_of(b"b"));
    }
}

#[cfg(test)]
mod pinned_verifier_tests {
    use std::sync::Arc;

    use super::{fingerprint_of, pinned_client_config, PinnedVerifier};
    use rustls::client::danger::ServerCertVerifier;
    use rustls::crypto::aws_lc_rs;
    // DigitallySignedStruct's constructor is pub(crate); decoding its wire form is the only way in.
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
        let certified = rcgen::generate_simple_self_signed(["localhost".to_string()])
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
        PinnedVerifier::new(fingerprint, &aws_lc_rs::default_provider())
    }

    /// Signs `message` with `key` the way a server's CertificateVerify does.
    fn sign(key: &PrivateKeyDer<'_>, message: &[u8]) -> DigitallySignedStruct {
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
        // CertifiedKey::new skips the cert/key match check, as an attacker's server would.
        let resolver = SingleCertAndKey::from(CertifiedKey::new(vec![cert.clone()], signing_key));
        let server_config = rustls::ServerConfig::builder_with_provider(provider)
            .with_protocol_versions(versions)?
            .with_no_client_auth()
            .with_cert_resolver(Arc::new(resolver));
        let mut server = rustls::ServerConnection::new(Arc::new(server_config))?;
        let mut client = rustls::ClientConnection::new(
            pinned_client_config(fingerprint_of(cert)),
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
            let mut inbound = to_server.as_slice();
            while !inbound.is_empty() {
                server.read_tls(&mut inbound).unwrap();
                server.process_new_packets()?;
            }
            let mut inbound = to_client.as_slice();
            while !inbound.is_empty() {
                client.read_tls(&mut inbound).unwrap();
                client.process_new_packets()?;
            }
        }
        Ok(())
    }

    #[test]
    fn handshake_signature_from_a_key_other_than_the_pinned_certs_is_rejected() {
        let (cert, _) = generate_identity();
        let verifier = verifier_pinned_to(fingerprint_of(&cert));
        let forged = sign(&attacker_key(), TRANSCRIPT);

        let tls13 = verifier.verify_tls13_signature(TRANSCRIPT, &cert, &forged);
        let tls12 = verifier.verify_tls12_signature(TRANSCRIPT, &cert, &forged);

        let bad_signature = TlsError::InvalidCertificate(CertificateError::BadSignature);
        assert!(
            matches!(&tls13, Err(e) if *e == bad_signature),
            "expected TLS 1.3 to reject an attacker-key signature, got {tls13:?}"
        );
        assert!(
            matches!(&tls12, Err(e) if *e == bad_signature),
            "expected TLS 1.2 to reject an attacker-key signature, got {tls12:?}"
        );
    }

    #[test]
    fn handshake_signature_from_the_pinned_certs_key_is_accepted() {
        let (cert, key) = generate_identity();
        let verifier = verifier_pinned_to(fingerprint_of(&cert));
        let genuine = sign(&key, TRANSCRIPT);

        let tls13 = verifier.verify_tls13_signature(TRANSCRIPT, &cert, &genuine);
        let tls12 = verifier.verify_tls12_signature(TRANSCRIPT, &cert, &genuine);

        assert!(tls13.is_ok(), "expected TLS 1.3 to accept, got {tls13:?}");
        assert!(tls12.is_ok(), "expected TLS 1.2 to accept, got {tls12:?}");
    }

    #[test]
    fn server_replaying_the_pinned_cert_without_its_key_fails_the_handshake() {
        let (cert, _) = generate_identity();

        let result = handshake(&cert, attacker_key(), rustls::DEFAULT_VERSIONS);

        assert_eq!(
            result,
            Err(TlsError::InvalidCertificate(CertificateError::BadSignature)),
            "expected the MITM handshake to fail on its forged signature"
        );
    }

    #[test]
    fn server_holding_the_pinned_certs_key_completes_the_handshake() {
        let (cert, key) = generate_identity();

        let result = handshake(&cert, key, rustls::DEFAULT_VERSIONS);

        assert_eq!(result, Ok(()));
    }

    #[test]
    fn server_offering_only_tls12_is_refused() {
        let (cert, key) = generate_identity();

        let result = handshake(&cert, key, &[&rustls::version::TLS12]);

        assert_eq!(
            result,
            Err(TlsError::PeerIncompatible(
                PeerIncompatible::Tls12NotOfferedOrEnabled
            )),
            "expected the pinned client not to offer TLS 1.2"
        );
    }

    #[test]
    fn verify_server_cert_accepts_the_matching_fingerprint() {
        let (der, _) = generate_identity();
        let verifier = verifier_pinned_to(fingerprint_of(&der));
        let server_name = ServerName::try_from("localhost").unwrap();

        let result = verifier.verify_server_cert(&der, &[], &server_name, &[], UnixTime::now());

        assert!(
            result.is_ok(),
            "expected the pinned (matching) fingerprint to verify, got {result:?}"
        );
    }

    #[test]
    fn verify_server_cert_rejects_a_wrong_fingerprint() {
        let (der, _) = generate_identity();
        let wrong_fingerprint = "SHA256:0000 0000 0000 0000 0000 0000".to_string();
        assert_ne!(
            fingerprint_of(&der),
            wrong_fingerprint,
            "test fixture bug: the 'wrong' fingerprint accidentally matches the real one"
        );
        let verifier = verifier_pinned_to(wrong_fingerprint);
        let server_name = ServerName::try_from("localhost").unwrap();

        let result = verifier.verify_server_cert(&der, &[], &server_name, &[], UnixTime::now());

        assert!(
            result.is_err(),
            "expected a mismatched fingerprint to be rejected, got {result:?}"
        );
    }
}
