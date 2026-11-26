use std::collections::BTreeMap;

use protocol::PaneId;
use serde::Deserialize;

use crate::layout::tab::pane_instance::PaneInstance;
use crate::space_id::SpaceId;
use crate::tab_id::TabId;
use crate::tree::split_node::SplitNode;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct TabRecord {
    pub(crate) id: TabId,
    pub(crate) title: String,
    pub(crate) space: SpaceId,
    pub(crate) panes: BTreeMap<PaneId, PaneInstance>,
    pub(crate) tree: Option<SplitNode>,
    pub(crate) focused: Option<PaneId>,
}

#[cfg(test)]
mod tests {
    use serde_json::{Value, json};

    use crate::layout::tab::Tab;

    fn leaf(member: &str) -> Value {
        json!({"node": "leaf", "chat": null, "tabs": [member], "active": member, "pinned": 0,
            "editorOpen": true, "chatSelected": false})
    }

    fn stored(
        tree: &Value,
        focused: &Value,
    ) -> Value {
        let pane = json!({"paneId": "a", "type": "core.terminal", "params": {}, "state": null});
        json!({"id": 1, "title": "t", "space": "", "panes": {"a": pane}, "tree": tree,
            "focused": focused})
    }

    #[test]
    fn only_a_record_that_keeps_the_tab_invariants_becomes_a_tab() {
        let cases = [
            ("consistent", stored(&leaf("a"), &json!("a")), true),
            ("tree missing", stored(&json!(null), &json!("a")), false),
            ("focus on nothing", stored(&leaf("a"), &json!(null)), false),
            ("focus elsewhere", stored(&leaf("a"), &json!("zz")), false),
            ("leaf without a pane", stored(&leaf("zz"), &json!("a")), false),
        ];
        for (name, record, loads) in cases {
            let tab = serde_json::from_value::<Tab>(record);
            assert_eq!(tab.map(|tab| tab.len()).ok(), loads.then_some(1), "{name}");
        }
    }
}
