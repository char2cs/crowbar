use super::*;

#[test]
fn identifiers_compare_by_text() {
    let from_str = ProjectId::from("a");
    let from_string = ProjectId::from(String::from("a"));
    assert_eq!(from_str, from_string, "same text, same id");
    assert_eq!(from_str.as_str(), "a", "text round trip");
    assert_ne!(from_str, ProjectId::from("b"), "different text");
}
