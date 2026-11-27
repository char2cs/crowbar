use protocol::{PaneId, PaneState, PaneTypeId, Params};
use serde::{Deserialize, Serialize};

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PaneInstance {
    pub pane_id: PaneId,
    #[serde(rename = "type")]
    pub type_id: PaneTypeId,
    pub params: Params,
    pub state: PaneState,
}
