//! Separators, sash, scroll area, drag handle and tooltip.

use gpui::{Context, IntoElement, ParentElement, Styled, div, px};
use ui::{Axis, Button, DragHandle, IconName, ScrollArea, Sash, Separator, Tooltip};

use crate::page::Page;

impl Page {
    pub(crate) fn structure(&self, _cx: &Context<Self>) -> impl IntoElement {
        let rows = (0..30).fold(div().flex().flex_col().gap(px(4.0)).p(px(8.0)), |column, index| {
            column.child(div().child(format!("Row {index}")))
        });
        div()
            .flex()
            .items_start()
            .gap(px(24.0))
            .child(div().flex().flex_col().gap(px(8.0)).w(px(160.0)).child(Separator::horizontal()).child(div().h(px(24.0)).flex().gap(px(8.0)).child(Separator::vertical())))
            .child(div().h(px(120.0)).w(px(8.0)).child(Sash::new("sash", Axis::Horizontal)))
            .child(div().h(px(120.0)).w(px(180.0)).child(ScrollArea::new("scroll", &self.scroll, rows)))
            .child(DragHandle::new("handle"))
            .child(Tooltip::new("tip", "Hide Sidebar", Button::icon_only("tip-trigger", IconName::Gear)).shortcut("Ctrl+B"))
    }
}
