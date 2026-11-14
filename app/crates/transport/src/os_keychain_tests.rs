use super::*;

#[test]
fn only_an_unknown_failure_class_is_remembered_as_a_denial() {
    let locked = keyring::Error::NoStorageAccess("locked".into());
    assert!(!is_denial(&locked));
    assert!(is_denial(&keyring::Error::NoEntry));
    assert!(is_denial(&keyring::Error::TooLong("x".into(), 1)));
}
