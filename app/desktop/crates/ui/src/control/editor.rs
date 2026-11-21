//! Text fields: the buffer, the edit history, the single-line input and the multi-line area.

mod buffer;
mod field;
mod history;
mod secondary;
#[cfg(test)]
mod test_support;
mod text_area;
mod text_input;
mod text_input_event;
mod text_input_style;

pub(crate) use text_area::bindings as text_area_bindings;
pub use text_area::{TextArea, TextAreaCondition, TextAreaSize};
pub use text_input::TextInput;
pub(crate) use text_input::bindings as text_input_bindings;
pub use text_input_event::TextInputEvent;
pub use text_input_style::TextInputStyle;
