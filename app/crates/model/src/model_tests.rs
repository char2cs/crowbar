use serde_json::json;

use super::*;
use crate::command::Command;
use crate::schema::SCHEMA_VERSION;
use crate::tab_id::TabId;

fn layout() -> Layout {
    let mut layout = Layout::default();
    layout
        .apply(Command::AddTab {
            id: TabId(1),
            title: "one".into(),
        })
        .unwrap();
    layout
}

#[test]
fn a_stored_layout_loads_back_identical() {
    let bytes = store(&layout()).unwrap();
    assert_eq!(load(&bytes).unwrap(), layout(), "round trip");
}

#[test]
fn the_first_unversioned_format_is_migrated_on_load() {
    let old = json!({"tabs": [{"id": 1, "title": "one", "panes": {}, "tree": null, "focused": null}], "active": 1});
    assert_eq!(
        load(old.to_string().as_bytes()).unwrap(),
        layout(),
        "migrated"
    );
}

#[test]
fn unreadable_newer_and_broken_data_are_distinct_errors() {
    assert!(
        matches!(load(b"not json"), Err(Error::Unreadable(_))),
        "syntax"
    );
    assert!(
        matches!(load(b"{\"tabs\": 3}"), Err(Error::Unreadable(_))),
        "shape"
    );
    let newer = json!({"version": SCHEMA_VERSION + 1, "tabs": [], "active": null});
    assert_eq!(
        load(newer.to_string().as_bytes()),
        Err(Error::UnsupportedSchema {
            found: SCHEMA_VERSION + 1,
            supported: SCHEMA_VERSION
        }),
        "newer"
    );
    let broken = json!({"version": 1, "tabs": [], "active": 4});
    assert!(
        matches!(load(broken.to_string().as_bytes()), Err(Error::Invalid(_))),
        "invariant"
    );
}
