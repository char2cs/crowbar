use std::sync::Arc;

use super::*;
use crate::fingerprint::{certificate_digest, fingerprint_of};
use rustls::client::danger::ServerCertVerifier;
use rustls::crypto::aws_lc_rs;
// DigitallySignedStruct's constructor is pub(crate); decoding its wire form is the only way in.
use rustls::internal::msgs::codec::Codec;
use rustls::pki_types::{CertificateDer, PrivateKeyDer, PrivatePkcs8KeyDer, ServerName, UnixTime};
use rustls::sign::{CertifiedKey, SingleCertAndKey};
use rustls::{
    CertificateError, DigitallySignedStruct, Error as TlsError, PeerIncompatible, SignatureScheme,
    SupportedProtocolVersion,
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
    let wrong_fingerprint = "SHA256:0000 0000 0000 0000 0000 0000".to_owned();
    assert_ne!(
        fingerprint_of(&der),
        wrong_fingerprint,
        "test fixture bug: the 'wrong' fingerprint accidentally matches the real one"
    );
    let verifier = verifier_pinned_to(wrong_fingerprint);
    let server_name = ServerName::try_from("localhost").unwrap();

    let result = verifier.verify_server_cert(&der, &[], &server_name, &[], UnixTime::now());

    assert_eq!(
        result.err(),
        Some(TlsError::InvalidCertificate(
            CertificateError::ApplicationVerificationFailure
        )),
        "expected a mismatched fingerprint to be rejected as an invalid certificate"
    );
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
