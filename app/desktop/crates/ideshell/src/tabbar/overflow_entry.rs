use gpui::SharedString;

use crate::tabbar::tab::tab_state::TabState;
use crate::tabbar::tab_actions::TabAction;

pub struct OverflowEntry {
    pub label: SharedString,
    pub state: TabState,
    pub select: TabAction,
}

ui::opaque_debug!(OverflowEntry);
