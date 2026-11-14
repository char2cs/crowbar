//! A button inside a row.

use gpui::IntoElement;
use ui::{Button, ButtonSize, ButtonVariant};

use crate::button_spec::ButtonSpec;
use crate::dispatch::send;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::icon_stem::icon_named;
use crate::intent::Intent;
use crate::row::Row;

pub(crate) fn button_control(frame: &Frame<'_>, row: &Row, spec: &ButtonSpec) -> impl IntoElement {
    let button = Button::new(element_id("button", row.key.as_str()), spec.label.clone())
        .variant(ButtonVariant::Outline)
        .size(ButtonSize::Sm)
        .on_click(send(frame.owner, Intent::Button(spec.clone())));
    let button = match spec.icon.as_deref().and_then(icon_named) {
        Some(icon) => button.icon(icon),
        None => button,
    };
    if frame.state.button_enabled(spec) {
        button
    } else {
        button.disabled()
    }
}

#[cfg(test)]
#[path = "button_control_tests.rs"]
mod tests;
