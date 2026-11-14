use super::*;

#[test]
fn starts_with_a_letter() {
    assert!(ExtensionId::new("git").is_ok(), "plain");
    assert!(ExtensionId::new("my-ext2").is_ok(), "digits and dashes");
    assert!(ExtensionId::new("2git").is_err(), "leading digit");
    assert!(ExtensionId::new("-git").is_err(), "leading dash");
    assert!(ExtensionId::new("Git").is_err(), "upper case");
}
