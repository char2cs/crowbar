//! The contract of a painted text field: text, selection, focus and editing, and how a field paints its caret and selection.

mod chrome;
mod field_editable;
mod field_focus;
mod field_selection;
mod field_text;
mod paint;
mod selection_drag;
mod text_frame;

pub(crate) use chrome::{Chrome, chrome};
pub(crate) use field_editable::FieldEditable;
pub(crate) use field_focus::FieldFocus;
pub(crate) use field_selection::FieldSelection;
pub(crate) use field_text::FieldText;
pub(crate) use paint::{band_frame, caret_color, caret_frame, runs_for};
pub(crate) use selection_drag::{SelectionDrag, capture_pointer};
pub(crate) use text_frame::TextFrame;
