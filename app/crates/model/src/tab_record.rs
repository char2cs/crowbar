use std::collections::BTreeMap;

use protocol::PaneId;
use serde::Deserialize;

use crate::error::Error;
use crate::pane_instance::PaneInstance;
use crate::split_node::SplitNode;
use crate::tab::Tab;
use crate::tab_id::TabId;
use crate::tab_shape::TabShape;

#[derive(Debug, Deserialize)]
#[serde(rename_all = "camelCase", deny_unknown_fields)]
pub(crate) struct TabRecord {
    id: TabId,
    title: String,
    panes: BTreeMap<PaneId, PaneInstance>,
    tree: Option<SplitNode>,
    focused: Option<PaneId>,
}

impl TryFrom<TabRecord> for Tab {
    type Error = Error;

    fn try_from(record: TabRecord) -> Result<Self, Error> {
        let shape = TabShape {
            id: record.id,
            title: record.title,
            tree: record.tree,
            focused: record.focused,
        };
        let tab = Self::restored(shape, record.panes);
        tab.validate()?;
        Ok(tab)
    }
}

#[cfg(test)]
#[path = "tab_record_tests.rs"]
mod tests;
