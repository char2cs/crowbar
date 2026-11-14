use super::*;

#[test]
fn a_credential_round_trips_and_hides_its_token_from_debug() {
    let credential = Credential {
        token: "secret-token".into(),
        pin: [7; 32],
    };
    assert_eq!(
        Credential::decode(&credential.encode()),
        Some(credential.clone())
    );
    assert!(!format!("{credential:?}").contains("secret-token"));
}

#[test]
fn anything_pairing_did_not_write_decodes_to_nothing() {
    let pin = "00".repeat(32);
    for stored in [
        "sometoken".to_owned(),
        String::new(),
        format!(r#"{{"token":"","pin":"{pin}"}}"#),
        format!(r#"{{"token":" \t","pin":"{pin}"}}"#),
        r#"{"token":"t","pin":"abcd"}"#.to_owned(),
        format!(r#"{{"token":"t","pin":"{}"}}"#, "zz".repeat(32)),
        format!(r#"{{"token":"t","pin":"{}é"}}"#, "0".repeat(63)),
        format!(r#"{{"pin":"{pin}"}}"#),
    ] {
        assert_eq!(Credential::decode(&stored), None, "{stored}");
    }
}
