use gpui::SharedString;

use crate::tab_actions::TabAction;
use crate::tab_state::TabState;
pub(crate) struct OverflowEntry {
    pub(crate) label: SharedString,
    pub(crate) state: TabState,
    pub(crate) select: TabAction,
}
