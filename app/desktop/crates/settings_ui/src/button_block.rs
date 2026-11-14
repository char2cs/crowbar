//! A button between blocks, such as Add host.

use gpui::{IntoElement, ParentElement, Styled, div, px};
use ui::{Button, ButtonSize, ButtonVariant};

use crate::action::Action;
use crate::button_spec::ButtonSpec;
use crate::dispatch::send;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::icon_stem::icon_named;
use crate::intent::Intent;

pub(crate) fn button_block(frame: &Frame<'_>, spec: &ButtonSpec) -> Option<impl IntoElement> {
    if matches!(spec.action, Action::ResetOverrides(_)) && !frame.state.has_custom_chords() {
        return None;
    }
    let button = Button::new(element_id("block", &spec.label), spec.label.clone())
        .variant(ButtonVariant::Outline)
        .size(ButtonSize::Sm)
        .on_click(send(frame.owner, Intent::Button(spec.clone())));
    let button = match spec.icon.as_deref().and_then(icon_named) {
        Some(icon) => button.icon(icon),
        None => button,
    };
    let button = if frame.state.button_enabled(spec) {
        button
    } else {
        button.disabled()
    };
    Some(div().px(px(4.0)).flex().child(button))
}

#[cfg(test)]
#[path = "button_block_tests.rs"]
mod tests;
