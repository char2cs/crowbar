use std::collections::BTreeMap;

use protocol::PaneId;
use serde::{Deserialize, Serialize};

use crate::error::Error;
use crate::pane_instance::PaneInstance;
use crate::space_id::SpaceId;
use crate::split_node::SplitNode;
use crate::tab_id::TabId;
use crate::tab_record::TabRecord;
use crate::tab_shape::TabShape;

/// Most panes one tab may hold; bounds the tree depth and the cost of every tree walk.
pub const MAX_PANES_PER_TAB: usize = 64;

/// A tab: its panes arranged in a split tree of groups, and which pane has focus.
///
/// Invariants, kept by every operation and checked on load: the groups at the tree's leaves
/// hold exactly the panes, each pane appears once, every group is non-empty and shows one of its
/// members, every split has two children, the focused pane exists and is the one its group
/// shows, and a tab without panes has no tree and no focus. A tab only changes through [`Layout::apply`].
///
/// [`Layout::apply`]: crate::Layout::apply
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase", try_from = "TabRecord")]
pub struct Tab {
    pub(crate) id: TabId,
    pub(crate) title: String,
    pub(crate) space: SpaceId,
    pub(crate) panes: BTreeMap<PaneId, PaneInstance>,
    pub(crate) tree: Option<SplitNode>,
    pub(crate) focused: Option<PaneId>,
}

impl Tab {
    pub(crate) fn new(
        id: TabId,
        title: impl Into<String>,
        space: SpaceId,
    ) -> Self {
        Self::restored(
            TabShape {
                id,
                title: title.into(),
                space,
                tree: None,
                focused: None,
            },
            BTreeMap::new(),
        )
    }

    pub(crate) fn restored(
        shape: TabShape,
        panes: BTreeMap<PaneId, PaneInstance>,
    ) -> Self {
        Self {
            id: shape.id,
            title: shape.title,
            space: shape.space,
            panes,
            tree: shape.tree,
            focused: shape.focused,
        }
    }

    pub(crate) fn from_shape(
        shape: TabShape,
        pool: &mut BTreeMap<PaneId, PaneInstance>,
    ) -> Self {
        let leaves = shape
            .tree
            .as_ref()
            .map_or_else(Vec::new, SplitNode::pane_ids);
        let panes = leaves
            .into_iter()
            .filter_map(|id| pool.remove(&id).map(|instance| (id, instance)))
            .collect();
        Self::restored(shape, panes)
    }

    /// The tab's id.
    #[must_use]
    pub fn id(&self) -> TabId {
        self.id
    }

    /// The tab's title.
    #[must_use]
    pub fn title(&self) -> &str {
        &self.title
    }

    /// The split tree, or `None` while the tab has no panes.
    #[must_use]
    pub fn tree(&self) -> Option<&SplitNode> {
        self.tree.as_ref()
    }

    /// The focused pane.
    #[must_use]
    pub fn focused(&self) -> Option<&PaneId> {
        self.focused.as_ref()
    }

    /// Looks up a pane.
    #[must_use]
    pub fn pane(
        &self,
        id: &PaneId,
    ) -> Option<&PaneInstance> {
        self.panes.get(id)
    }

    /// Number of panes.
    #[must_use]
    pub fn len(&self) -> usize {
        self.panes.len()
    }

    /// Whether the tab has no panes.
    #[must_use]
    pub fn is_empty(&self) -> bool {
        self.panes.is_empty()
    }

    /// Pane ids in layout order (left to right, top to bottom).
    #[must_use]
    pub fn pane_ids(&self) -> Vec<PaneId> {
        self.tree
            .as_ref()
            .map_or_else(Vec::new, SplitNode::pane_ids)
    }
}

impl TryFrom<TabRecord> for Tab {
    type Error = Error;

    fn try_from(record: TabRecord) -> Result<Self, Error> {
        let shape = TabShape {
            id: record.id,
            title: record.title,
            space: record.space,
            tree: record.tree,
            focused: record.focused,
        };
        let tab = Self::restored(shape, record.panes);
        tab.validate()?;
        Ok(tab)
    }
}

#[cfg(test)]
mod tests {
    use serde_json::json;

    use super::*;
    use crate::test_support::tab_fixtures::tab_with;

    #[test]
    fn deserializing_a_tab_checks_its_invariants() {
        let tab = tab_with(&["a", "b"]);
        let mut value = serde_json::to_value(&tab).unwrap();
        assert_eq!(
            serde_json::from_value::<Tab>(value.clone()).unwrap(),
            tab,
            "round trip"
        );
        value["focused"] = json!("ghost");
        assert!(
            serde_json::from_value::<Tab>(value.clone()).is_err(),
            "bad focus"
        );
        value["focused"] = json!("a");
        value["extra"] = json!(1);
        assert!(
            serde_json::from_value::<Tab>(value).is_err(),
            "unknown field"
        );
    }
}
