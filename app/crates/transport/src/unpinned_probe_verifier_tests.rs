use rustls::crypto::aws_lc_rs;
// DigitallySignedStruct's constructor is pub(crate); decoding its wire form is the only way in.
use rustls::internal::msgs::codec::Codec;

use super::*;

fn verifier() -> UnpinnedProbeVerifier {
    UnpinnedProbeVerifier {
        algorithms: aws_lc_rs::default_provider().signature_verification_algorithms,
    }
}

#[test]
fn any_certificate_is_accepted_because_the_probe_sends_nothing() {
    let certified = rcgen::generate_simple_self_signed(["anywhere.example".to_owned()]).unwrap();
    let cert = certified.cert.der().clone();
    let name = ServerName::try_from("localhost").unwrap();
    let verdict = verifier().verify_server_cert(&cert, &[], &name, &[], UnixTime::now());
    assert!(verdict.is_ok());
}

#[test]
fn a_handshake_signature_is_still_checked_so_the_host_must_hold_the_key() {
    let certified = rcgen::generate_simple_self_signed(["localhost".to_owned()]).unwrap();
    let cert = certified.cert.der().clone();
    let mut wire = SignatureScheme::ECDSA_NISTP256_SHA256.to_array().to_vec();
    wire.extend_from_slice(&64u16.to_be_bytes());
    wire.extend_from_slice(&[0u8; 64]);
    let forged = DigitallySignedStruct::read_bytes(&wire).unwrap();
    assert!(
        verifier()
            .verify_tls13_signature(b"message", &cert, &forged)
            .is_err()
    );
}

#[test]
fn it_offers_only_the_signature_scheme_it_can_check() {
    assert_eq!(
        verifier().supported_verify_schemes(),
        vec![SignatureScheme::ECDSA_NISTP256_SHA256]
    );
}
