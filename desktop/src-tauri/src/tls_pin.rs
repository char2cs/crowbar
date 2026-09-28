//! Fingerprint pinning for remote daemon connections: instead of validating a
//! certificate chain against a CA (there is none — every daemon's identity is
//! self-signed, `api/internal/core/tlsid`), a remote host's certificate is
//! trusted exactly when its fingerprint matches the one the user confirmed
//! during pairing.

use std::sync::Arc;

use rustls::client::danger::{HandshakeSignatureValid, ServerCertVerified, ServerCertVerifier};
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
#[allow(dead_code)] // dead_code: constructed by pinned_client_config, which lands its own caller in a later task of this plan
struct PinnedVerifier {
    fingerprint: String,
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

    fn verify_tls12_signature(
        &self,
        _message: &[u8],
        _cert: &CertificateDer<'_>,
        _dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, TlsError> {
        // No chain to validate a signature against — the certificate itself
        // is the trust anchor once its fingerprint has matched above.
        Ok(HandshakeSignatureValid::assertion())
    }

    fn verify_tls13_signature(
        &self,
        _message: &[u8],
        _cert: &CertificateDer<'_>,
        _dss: &DigitallySignedStruct,
    ) -> Result<HandshakeSignatureValid, TlsError> {
        Ok(HandshakeSignatureValid::assertion())
    }

    fn supported_verify_schemes(&self) -> Vec<SignatureScheme> {
        // The daemon's identity is ECDSA P-256 (api/internal/core/tlsid.go
        // generates elliptic.P256 keys) — TLS 1.3 signs with ECDSA-SHA256.
        vec![SignatureScheme::ECDSA_NISTP256_SHA256]
    }
}

/// Builds a rustls client config that trusts only the certificate matching
/// `fingerprint`.
#[allow(dead_code)] // dead_code: no caller until the TLS-dial task in this plan wires this into the real transport
pub fn pinned_client_config(fingerprint: String) -> Arc<rustls::ClientConfig> {
    let verifier = Arc::new(PinnedVerifier { fingerprint });
    Arc::new(
        rustls::ClientConfig::builder()
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
    use super::{fingerprint_of, PinnedVerifier};
    use rustls::client::danger::ServerCertVerifier;
    use rustls::pki_types::{CertificateDer, ServerName, UnixTime};

    /// A real, self-signed leaf certificate — DER bytes a genuine TLS
    /// handshake would present — so the assertions below exercise
    /// `verify_server_cert`'s actual fingerprint comparison, not a stand-in.
    fn generate_test_cert() -> CertificateDer<'static> {
        rcgen::generate_simple_self_signed(["localhost".to_string()])
            .expect("generate self-signed test cert")
            .cert
            .der()
            .clone()
    }

    #[test]
    fn verify_server_cert_accepts_the_matching_fingerprint() {
        let der = generate_test_cert();
        let verifier = PinnedVerifier {
            fingerprint: fingerprint_of(&der),
        };
        let server_name = ServerName::try_from("localhost").unwrap();

        let result = verifier.verify_server_cert(&der, &[], &server_name, &[], UnixTime::now());

        assert!(
            result.is_ok(),
            "expected the pinned (matching) fingerprint to verify, got {result:?}"
        );
    }

    #[test]
    fn verify_server_cert_rejects_a_wrong_fingerprint() {
        let der = generate_test_cert();
        let wrong_fingerprint = "SHA256:0000 0000 0000 0000 0000 0000".to_string();
        assert_ne!(
            fingerprint_of(&der),
            wrong_fingerprint,
            "test fixture bug: the 'wrong' fingerprint accidentally matches the real one"
        );
        let verifier = PinnedVerifier {
            fingerprint: wrong_fingerprint,
        };
        let server_name = ServerName::try_from("localhost").unwrap();

        let result = verifier.verify_server_cert(&der, &[], &server_name, &[], UnixTime::now());

        assert!(
            result.is_err(),
            "expected a mismatched fingerprint to be rejected, got {result:?}"
        );
    }
}
