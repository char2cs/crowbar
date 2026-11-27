pub(super) mod open_mode;

use serde::{Deserialize, Serialize};

use crate::control::payload::{DocumentPath, OpenMode, Params};
use crate::identity::PaneTypeId;

#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PaneOpen {
    pub type_id: PaneTypeId,
    pub params: Params,
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub file: Option<DocumentPath>,
    #[serde(default)]
    pub mode: OpenMode,
}
