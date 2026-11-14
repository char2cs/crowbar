//! Separators, sash, scroll area, drag handle and tooltip.

use gpui::{IntoElement, ParentElement, ScrollHandle, Styled, div, px};
use ui::{Axis, Button, DragHandle, IconName, Sash, ScrollArea, Separator, Tooltip};

fn rows() -> gpui::Div {
    let lines = (0..30).map(|index| div().child(format!("Row {index}")));
    div()
        .flex()
        .flex_col()
        .gap(px(4.0))
        .p(px(8.0))
        .children(lines)
}

fn separators() -> impl IntoElement {
    let vertical = div()
        .h(px(24.0))
        .flex()
        .gap(px(8.0))
        .child(Separator::vertical());
    div()
        .flex()
        .flex_col()
        .gap(px(8.0))
        .w(px(160.0))
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
        .gap(px(24.0))
        .child(separators())
        .child(
            div()
                .h(px(120.0))
                .w(px(8.0))
                .child(Sash::new("sash", Axis::Horizontal)),
        )
        .child(
            div()
                .h(px(120.0))
                .w(px(180.0))
                .child(ScrollArea::new("scroll", scroll, rows())),
        )
        .child(DragHandle::new("handle"))
        .child(tip)
}
