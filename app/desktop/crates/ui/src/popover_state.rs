//! Whether a popover is open and the focus handle of its panel.

use gpui::FocusHandle;

#[derive(Debug)]
pub(crate) struct PopoverState {
    pub(crate) open: bool,
    pub(crate) focus: FocusHandle,
}
