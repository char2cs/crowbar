use super::*;
use crate::control::Control;
use crate::section::Section;
use crate::value::Value;

fn page(id: &str, key: &str) -> Page {
    Page::new(id, id, "gear")
        .block(Section::new("S").row(Row::new(key, key, Control::switch()).default_value(true)))
}

#[test]
fn defaults_come_from_the_rows() {
    let schema = Schema::new(vec![page("a", "x"), page("b", "y")]).unwrap();
    assert_eq!(
        schema.defaults().get(&Key::from("y")),
        Some(&Value::Bool(true))
    );
    assert_eq!(schema.pages().len(), 2);
    assert!(schema.page(&PageId::from("b")).is_some());
    assert_eq!(schema.rows().count(), 2);
}

#[test]
fn repeated_pages_and_keys_are_rejected() {
    let same_page = Schema::new(vec![page("a", "x"), page("a", "y")]);
    assert!(
        matches!(same_page, Err(Error::Duplicate { what: "page", .. })),
        "{same_page:?}"
    );
    let same_key = Schema::new(vec![page("a", "x"), page("b", "x")]);
    assert!(
        matches!(same_key, Err(Error::Duplicate { what: "key", .. })),
        "{same_key:?}"
    );
}

#[test]
fn extension_pages_stay_in_their_namespace() {
    let mut schema = Schema::new(vec![page("core", "x")]).unwrap();
    schema
        .extend("git", vec![page("ext.git", "ext.git.autoFetch")])
        .unwrap();
    assert_eq!(schema.pages().len(), 2);
    let stealing = schema.extend("git", vec![page("ext.git.more", "x2")]);
    assert!(
        matches!(stealing, Err(Error::OutsideNamespace { .. })),
        "{stealing:?}"
    );
    let other_page = schema.extend("git", vec![page("ext.gitlab", "ext.git.k")]);
    assert!(
        matches!(other_page, Err(Error::OutsideNamespace { .. })),
        "{other_page:?}"
    );
    let repeated = schema.extend("git", vec![page("ext.git", "ext.git.other")]);
    assert!(
        matches!(repeated, Err(Error::Duplicate { .. })),
        "{repeated:?}"
    );
    assert_eq!(
        schema.pages().len(),
        2,
        "a failed extension leaves the schema unchanged"
    );
}
