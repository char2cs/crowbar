//! Separators, sash, scroll area, drag handle and tooltip.

use gpui::{IntoElement, ParentElement, Pixels, ScrollHandle, Styled, div, px};
use ui::{Axis, Button, DragHandle, IconName, Sash, ScrollArea, Separator, Tooltip, spacing};

const ROW_COUNT: usize = 30;
const SEPARATORS_WIDTH: Pixels = px(160.0);
const PANEL_HEIGHT: Pixels = px(120.0);
const SCROLL_WIDTH: Pixels = px(180.0);

fn rows() -> gpui::Div {
    let lines = (0..ROW_COUNT).map(|index| div().child(format!("Row {index}")));
    div()
        .flex()
        .flex_col()
        .gap(spacing::STEP_1)
        .p(spacing::STEP_2)
        .children(lines)
}

fn separators() -> impl IntoElement {
    let vertical = div()
        .h(spacing::STEP_6)
        .flex()
        .gap(spacing::STEP_2)
        .child(Separator::vertical());
    div()
        .flex()
        .flex_col()
        .gap(spacing::STEP_2)
        .w(SEPARATORS_WIDTH)
        .child(Separator::horizontal())
        .child(vertical)
}

pub(crate) fn structure(scroll: &ScrollHandle) -> impl IntoElement {
    let tip = Tooltip::new(
        "tip",
        "Hide Sidebar",
        Button::icon_only("tip-trigger", IconName::Gear),
    )
    .shortcut("Ctrl+B");
    div()
        .flex()
        .items_start()
        .gap(spacing::STEP_6)
        .child(separators())
        .child(
            div()
                .h(PANEL_HEIGHT)
                .w(spacing::STEP_2)
                .child(Sash::new("sash", Axis::Horizontal)),
        )
        .child(div().h(PANEL_HEIGHT).w(SCROLL_WIDTH).child(ScrollArea::new(
            "scroll",
            scroll,
            rows(),
        )))
        .child(DragHandle::new("handle"))
        .child(tip)
}
