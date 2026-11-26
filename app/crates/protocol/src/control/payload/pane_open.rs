pub(super) mod open_mode;

use serde::{Deserialize, Serialize};

use crate::control::payload::{DocumentPath, OpenMode, Params};
use crate::identity::PaneTypeId;

/// What a page's `pane.open` asks the shell to open.
#[derive(Debug, Clone, PartialEq, Serialize, Deserialize)]
#[serde(rename_all = "camelCase")]
pub struct PaneOpen {
    /// The type to open.
    pub type_id: PaneTypeId,
    /// The ids the new pane is identified by.
    pub params: Params,
    /// The document an editor pane opens; the params name any other type of pane.
    #[serde(default, skip_serializing_if = "Option::is_none")]
    pub file: Option<DocumentPath>,
    /// Whether it opens as a preview; an ordinary tab when the page leaves it out.
    #[serde(default)]
    pub mode: OpenMode,
}
