use super::*;

#[test]
fn rejects_blank_tokens() {
    assert!(HostToken::parse("  ").is_none());
}
