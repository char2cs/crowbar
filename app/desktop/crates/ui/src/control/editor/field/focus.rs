//! The focus handle the input method attaches to.

use gpui::FocusHandle;

pub(crate) trait Focus {
    fn input_focus(&self) -> FocusHandle;
}
