use super::*;

const DER: &[u8] = b"not a real certificate, only bytes to hash";

#[test]
fn a_confirmed_pin_matches_the_displayed_fingerprint_of_the_certificate() {
    let pin = Pin::Confirmed(fingerprint_of(DER));
    assert!(pin.matches(DER));
    assert!(!pin.matches(b"another certificate"));
}

#[test]
fn a_paired_pin_matches_the_whole_digest() {
    let pin = Pin::Paired(certificate_digest(DER));
    assert!(pin.matches(DER));
    assert!(!pin.matches(b"another certificate"));
}

#[test]
fn both_pins_display_as_the_same_fingerprint() {
    let confirmed = Pin::Confirmed(fingerprint_of(DER));
    let paired = Pin::Paired(certificate_digest(DER));
    assert_eq!(confirmed.to_string(), paired.to_string());
}
