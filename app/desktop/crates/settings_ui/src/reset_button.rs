//! The reset icon of a row whose value differs from its default.

use gpui::{
    App, InteractiveElement, IntoElement, ParentElement, StatefulInteractiveElement, Styled, div,
    px,
};
use ui::{Icon, IconName, Surface, Theme, Tooltip, radius};

use crate::dispatch::send;
use crate::element_id::element_id;
use crate::frame::Frame;
use crate::intent::Intent;
use crate::row::Row;

const BOX: f32 = 20.0;
const ICON: f32 = 16.0;

pub(crate) fn reset_button(frame: &Frame<'_>, row: &Row, cx: &App) -> impl IntoElement {
    let slot = div().flex_none().size(px(BOX));
    if !frame.state.resettable(row) {
        return slot.into_any_element();
    }
    let palette = Theme::of(cx).palette(Surface::Window);
    let id = element_id("reset", row.key.as_str());
    let button = div()
        .id(id.clone())
        .flex()
        .items_center()
        .justify_center()
        .size(px(BOX))
        .rounded(radius::MD)
        .cursor_pointer()
        .hover(move |style| style.bg(palette.accent()))
        .on_click(send(frame.owner, Intent::Reset(row.clone())))
        .child(
            Icon::new(IconName::ArrowCounterClockwise)
                .size(px(ICON))
                .color(palette.muted_foreground()),
        );
    slot.child(Tooltip::new(id, format!("Reset {}", row.label), button))
        .into_any_element()
}

#[cfg(test)]
#[path = "reset_button_tests.rs"]
mod tests;
