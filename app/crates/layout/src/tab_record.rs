use std::collections::BTreeMap;

use protocol::PaneId;
use serde::Deserialize;

use crate::pane_instance::PaneInstance;
use crate::split_node::SplitNode;
use crate::tab_id::TabId;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct TabRecord {
    pub(crate) id: TabId,
    pub(crate) title: String,
    pub(crate) panes: BTreeMap<PaneId, PaneInstance>,
    pub(crate) tree: Option<SplitNode>,
    pub(crate) focused: Option<PaneId>,
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use crate::tab::Tab;

    fn stored(
        tree: &serde_json::Value,
        focused: &serde_json::Value,
    ) -> serde_json::Value {
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
        let record = stored(
            &json!({"node": "leaf", "chat": null, "tabs": ["a"], "active": "a", "pinned": 0, "editorOpen": true, "chatSelected": false}),
            &json!("a"),
        );
        let tab: Tab = serde_json::from_value(record).unwrap();
        assert_eq!(tab.len(), 1);
    }

    #[test]
    fn a_record_that_breaks_a_tab_invariant_is_refused() {
        let leaf = json!({"node": "leaf", "chat": null, "tabs": ["a"], "active": "a", "pinned": 0, "editorOpen": true, "chatSelected": false});
        let cases = [
            ("tree missing", stored(&json!(null), &json!("a"))),
            ("focus on nothing", stored(&leaf, &json!(null))),
            ("focus elsewhere", stored(&leaf, &json!("zz"))),
            (
                "leaf without a pane",
                stored(
                    &json!({"node": "leaf", "chat": null, "tabs": ["zz"], "active": "zz", "pinned": 0, "editorOpen": true, "chatSelected": false}),
                    &json!("a"),
                ),
            ),
        ];
        for (name, record) in cases {
            assert!(serde_json::from_value::<Tab>(record).is_err(), "{name}");
        }
    }
}
