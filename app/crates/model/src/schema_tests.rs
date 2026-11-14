use serde_json::json;

use super::*;

#[test]
fn an_unversioned_document_is_stamped_with_the_first_version() {
    let upgraded = upgrade(json!({"tabs": [], "active": null})).unwrap();
    assert_eq!(
        upgraded,
        json!({"version": 1, "tabs": [], "active": null}),
        "migrated"
    );
}

#[test]
fn a_current_document_is_left_alone() {
    let current = json!({"version": SCHEMA_VERSION, "tabs": [], "active": null});
    assert_eq!(upgrade(current.clone()).unwrap(), current, "unchanged");
}

#[test]
fn a_newer_document_is_refused_rather_than_misread() {
    assert_eq!(
        upgrade(json!({"version": SCHEMA_VERSION + 1, "tabs": []})),
        Err(Error::UnsupportedSchema {
            found: SCHEMA_VERSION + 1,
            supported: SCHEMA_VERSION
        }),
        "newer"
    );
}

#[test]
fn malformed_documents_are_refused() {
    for bad in [
        json!([]),
        json!("x"),
        json!({"version": "one"}),
        json!({"version": -1}),
        json!({"version": 1.5}),
    ] {
        assert!(
            matches!(upgrade(bad.clone()), Err(Error::Invalid(_))),
            "{bad}"
        );
    }
}
