//! A text or list field.

use gpui::{IntoElement, ParentElement, Styled, div, px};

use crate::control::Control;
use crate::frame::Frame;
use crate::row::Row;

pub(crate) fn field_control(frame: &Frame<'_>, row: &Row) -> impl IntoElement {
    let width = match &row.control {
        Control::Text { width, .. } | Control::TextList { width, .. } => width.pixels(),
        _ => 0.0,
    };
    div()
        .w(px(width))
        .max_w_full()
        .children(frame.fields.get(&row.key).cloned())
}

#[cfg(test)]
#[path = "field_control_tests.rs"]
mod tests;
