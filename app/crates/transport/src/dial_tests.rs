use super::*;

#[test]
fn a_certificate_the_verifier_rejected_is_an_identity_change_not_an_outage() {
    let host = RemoteHost {
        endpoint: crate::endpoint::Endpoint::parse("1.2.3.4:1").unwrap(),
        pin: crate::pin::Pin::Paired([0; 32]),
    };
    let rejected = io::Error::other(rustls::Error::InvalidCertificate(
        rustls::CertificateError::ApplicationVerificationFailure,
    ));
    let broken = io::Error::new(io::ErrorKind::ConnectionReset, "reset");
    assert!(matches!(
        handshake_failure(&host, &rejected),
        Error::IdentityChanged(_)
    ));
    assert!(matches!(
        handshake_failure(&host, &broken),
        Error::HostUnreachable(_)
    ));
}
