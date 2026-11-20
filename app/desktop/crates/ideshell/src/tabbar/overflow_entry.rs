use gpui::SharedString;

use crate::tabbar::tab_actions::TabAction;
use crate::tabbar::tab_state::TabState;

/// One row of the tab bar's overflow menu.
pub struct OverflowEntry {
    /// The tab's text.
    pub label: SharedString,
    /// Whether the row is the shown tab, which the menu checks.
    pub state: TabState,
    /// What choosing the row does.
    pub select: TabAction,
}

ui::opaque_debug!(OverflowEntry);
