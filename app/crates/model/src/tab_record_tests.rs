use serde_json::json;

use super::*;

fn stored(tree: &serde_json::Value, focused: &serde_json::Value) -> serde_json::Value {
    json!({
        "id": 1,
        "title": "t",
        "panes": {"a": {"paneId": "a", "type": "core.terminal", "params": {}, "state": null}},
        "tree": tree,
        "focused": focused,
    })
}

#[test]
fn a_consistent_record_becomes_a_tab() {
    let record = stored(&json!({"node": "leaf", "pane": "a"}), &json!("a"));
    let tab: Tab = serde_json::from_value(record).unwrap();
    assert_eq!(tab.len(), 1);
}

#[test]
fn a_record_that_breaks_a_tab_invariant_is_refused() {
    let leaf = json!({"node": "leaf", "pane": "a"});
    let cases = [
        ("tree missing", stored(&json!(null), &json!("a"))),
        ("focus on nothing", stored(&leaf, &json!(null))),
        ("focus elsewhere", stored(&leaf, &json!("zz"))),
        (
            "leaf without a pane",
            stored(&json!({"node": "leaf", "pane": "zz"}), &json!("a")),
        ),
    ];
    for (name, record) in cases {
        assert!(serde_json::from_value::<Tab>(record).is_err(), "{name}");
    }
}
