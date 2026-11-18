//! Whether a popover is open and the focus handle of its panel.

use gpui::FocusHandle;

#[derive(Debug)]
pub(crate) struct State {
    pub(crate) open: bool,
    pub(crate) focus: FocusHandle,
}
