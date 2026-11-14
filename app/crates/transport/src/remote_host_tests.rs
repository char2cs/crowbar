use super::*;
use crate::fingerprint::certificate_digest;

#[test]
fn a_paired_host_is_pinned_to_the_digest_stored_with_its_credential() {
    let credential = Credential {
        token: "t".into(),
        pin: certificate_digest(b"cert"),
    };
    let host = RemoteHost::paired(Endpoint::parse("10.0.0.2:7000").unwrap(), &credential);
    assert_eq!(host.pin, Pin::Paired(credential.pin));
}
