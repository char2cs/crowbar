use transport::{connect, HostToken};

#[test]
fn public_api_connects() {
    let token = HostToken::parse("ok").expect("token");
    assert!(connect(&token).is_ok());
}
