//! The contract of a painted text field: text, selection, focus and editing, and how a field paints its caret and selection.

mod blink;
mod blinking;
mod chrome;
mod editable;
mod focus;
mod frame;
mod paint;
mod selection;
mod selection_drag;
mod text;

pub(crate) use blink::{Blink, blinks, tend};
pub(crate) use blinking::Blinking;
pub(crate) use chrome::{Chrome, chrome};
pub(crate) use editable::Editable;
pub(crate) use focus::Focus;
pub(crate) use frame::Frame;
pub(crate) use paint::{band_frame, caret_frame, runs_for};
pub(crate) use selection::Selection;
pub(crate) use selection_drag::{SelectionDrag, capture_pointer};
pub(crate) use text::Text;
