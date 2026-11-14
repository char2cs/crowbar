use super::*;

#[test]
fn accepts_a_valid_token() {
    let token = HostToken::parse("ok").unwrap();
    assert!(connect(&token).is_ok());
}
