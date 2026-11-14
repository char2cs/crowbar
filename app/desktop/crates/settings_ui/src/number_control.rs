//! A stepper: minus, a typed field and plus.

use gpui::{IntoElement, ParentElement, Styled, div, px};
use ui::{Button, IconName};

use crate::control::Control;
use crate::dispatch::send;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::nudge::Nudge;
use crate::row::Row;

pub(crate) fn number_control(frame: &Frame<'_>, row: &Row) -> impl IntoElement {
    let Control::Number { range, width } = &row.control else {
        return div();
    };
    let typed = range.parse(&frame.state.field_text(row));
    let at_min = typed.is_some_and(|value| value <= range.min);
    let at_max = typed.is_some_and(|value| value >= range.max);
    let step = |name: &str, icon: IconName, nudge: Nudge, blocked: bool| {
        let button = Button::icon_only(element_id(name, row.key.as_str()), icon)
            .on_click(send(frame.owner, Intent::Nudge(row.clone(), nudge)));
        if blocked { button.disabled() } else { button }
    };
    div()
        .flex()
        .items_center()
        .gap(px(4.0))
        .w(px(width.pixels()))
        .child(step("minus", IconName::Minus, Nudge::Down, at_min))
        .child(
            div()
                .flex_1()
                .min_w_0()
                .children(frame.fields.get(&row.key).cloned()),
        )
        .child(step("plus", IconName::Plus, Nudge::Up, at_max))
}

#[cfg(test)]
#[path = "number_control_tests.rs"]
mod tests;
