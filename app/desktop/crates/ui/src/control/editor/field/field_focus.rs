//! The focus handle the input method attaches to.

use gpui::FocusHandle;

pub(crate) trait FieldFocus {
    fn input_focus(&self) -> FocusHandle;
}
