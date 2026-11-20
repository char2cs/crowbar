//! The IDE shell inside one pane: the top row and the tab bar.
//!
//! Every element here is drawn from data and action callbacks and decides nothing; the layout
//! view builds them for each pane group and wires the callbacks to its own commands.

mod tabbar;
mod toprow;

pub use tabbar::chat_tab::ChatTab;
pub use tabbar::overflow_entry::OverflowEntry;
pub use tabbar::pane_strip::{PaneStrip, STRIP_HEIGHT};
pub use tabbar::path_action::PathAction;
pub use tabbar::strip_tab::StripTab;
pub use tabbar::tab_actions::{StripKeys, TabAction, TabActions, TabDrop};
pub use tabbar::tab_end::TabEnd;
pub use tabbar::tab_look::TabLook;
pub use tabbar::tab_state::TabState;
pub use toprow::split_toggle::SplitToggle;
