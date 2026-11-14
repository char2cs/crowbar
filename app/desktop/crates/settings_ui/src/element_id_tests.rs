use super::*;

#[test]
fn ids_join_prefix_and_name_and_differ_by_either() {
    assert_eq!(element_id("row", "a"), element_id("row", "a"));
    assert_ne!(element_id("row", "a"), element_id("row", "b"));
    assert_ne!(element_id("row", "a"), element_id("reset", "a"));
}
