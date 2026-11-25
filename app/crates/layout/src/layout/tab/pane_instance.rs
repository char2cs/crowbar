use protocol::{PaneId, PaneState, PaneTypeId, Params};
use serde::{Deserialize, Serialize};

/// One pane of a tab. Its type is looked up in the pane registry; the model never matches on
/// it.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PaneInstance {
    /// Stable id, kept while the pane is parked, moved or restored.
    pub pane_id: PaneId,
    /// The registered pane type.
    #[serde(rename = "type")]
    pub type_id: PaneTypeId,
    /// The ids the pane is identified by.
    pub params: Params,
    /// The pane's persisted state.
    pub state: PaneState,
}
