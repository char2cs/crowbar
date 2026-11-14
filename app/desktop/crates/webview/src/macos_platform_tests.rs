use super::*;

#[test]
fn extensions_get_distinct_stable_store_identifiers() {
    let git = ExtensionId::new("git").unwrap();
    let draw = ExtensionId::new("draw").unwrap();
    assert_eq!(store_identifier(&git), store_identifier(&git), "stable");
    assert_ne!(store_identifier(&git), store_identifier(&draw), "distinct");
}
