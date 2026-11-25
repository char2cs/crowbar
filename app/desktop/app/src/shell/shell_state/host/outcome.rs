use protocol::{CommandId, PaneId};

use crate::panes::content::pane_toast_change::PaneToastChange;
use crate::panes::content::review_panes;
use crate::panes::editor::document_request;

#[derive(Debug, Default, PartialEq)]
pub(crate) struct Outcome {
    pub(crate) titles_changed: bool,
    pub(crate) dirty_changed: bool,
    pub(crate) hover: Vec<(PaneId, bool)>,
    pub(crate) focused: Vec<PaneId>,
    pub(crate) opens: Vec<layout::Command>,
    pub(crate) commands: Vec<CommandId>,
    pub(crate) pages: Vec<(PaneId, String)>,
    pub(crate) loaded: Vec<PaneId>,
    pub(crate) picks: Vec<(PaneId, browser::PageUpdate)>,
    pub(crate) settings: Vec<(protocol::SettingKey, serde_json::Value)>,
    pub(crate) pointer: Option<(PaneId, f64, f64)>,
    pub(crate) mounted: Vec<PaneId>,
    pub(crate) documents: Vec<document_request::DocumentRequest>,
    pub(crate) viewed: Vec<(PaneId, review_panes::ViewedChanges)>,
    pub(crate) toasts: Vec<PaneToastChange>,
}
