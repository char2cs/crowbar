//! Draws the control of a row.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};
use ui::font::{MONO_FAMILY, UI_FAMILY};
use ui::{Surface, Switch, Theme, UiText};

use crate::button_control::button_control;
use crate::control::Control;
use crate::dispatch::deliver;
use crate::element_id::element_id;
use crate::field_control::field_control;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::number_control::number_control;
use crate::row::Row;
use crate::select_control::select_control;

pub(crate) fn control_view(
    frame: &Frame<'_>,
    row: &Row,
    (window, cx): (&mut Window, &mut App),
) -> AnyElement {
    match &row.control {
        Control::Switch { .. } => switch_control(frame, row).into_any_element(),
        Control::Select { .. } => select_control(frame, row).into_any_element(),
        Control::Number { .. } => number_control(frame, row).into_any_element(),
        Control::Text { .. } | Control::TextList { .. } => {
            field_control(frame, row).into_any_element()
        },
        Control::Button(spec) => button_control(frame, row, spec).into_any_element(),
        Control::Readout { mono } => {
            readout_control(frame, (row, *mono), (window, cx)).into_any_element()
        },
    }
}

fn switch_control(frame: &Frame<'_>, row: &Row) -> impl IntoElement {
    let owner = frame.owner.clone();
    let target = row.clone();
    Switch::new(
        element_id("switch", row.key.as_str()),
        frame.state.switch_on(row),
    )
    .on_change(move |_, window, cx| {
        deliver(&owner, Intent::Toggle(target.clone()), (window, cx));
    })
}

fn readout_control(
    frame: &Frame<'_>,
    (row, mono): (&Row, bool),
    (_, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let theme = *Theme::of(cx);
    let family = if mono { MONO_FAMILY } else { UI_FAMILY };
    div()
        .max_w(px(320.0))
        .font_family(family)
        .text_size(UiText::Sm.size(&theme))
        .text_color(theme.palette(Surface::Window).muted_foreground())
        .child(frame.state.field_text(row))
}

#[cfg(test)]
#[path = "control_view_tests.rs"]
mod tests;
