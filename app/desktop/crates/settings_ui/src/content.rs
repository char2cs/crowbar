//! The scrolling page on the right.

use gpui::{AnyElement, App, IntoElement, ParentElement, ScrollHandle, Styled, Window, div, px};
use ui::ScrollArea;

use crate::block_view::block_view;
use crate::empty_view::empty_view;
use crate::frame::Frame;

pub(crate) fn content(
    frame: &Frame<'_>,
    scroll: &ScrollHandle,
    (window, cx): (&mut Window, &mut App),
) -> impl IntoElement {
    let mut blocks: Vec<AnyElement> = Vec::new();
    if let Some(page) = frame.state.page() {
        for (index, block) in page.blocks.iter().enumerate() {
            blocks.extend(block_view(frame, (index, block), (&mut *window, &mut *cx)));
        }
    }
    let page = div()
        .flex()
        .flex_col()
        .gap(px(16.0))
        .p(px(12.0))
        .children(blocks);
    let inner = if frame.state.page().is_some() {
        page.into_any_element()
    } else {
        empty_view(cx).into_any_element()
    };
    div()
        .flex_1()
        .min_w_0()
        .h_full()
        .child(ScrollArea::new("settings-page", scroll, inner))
}

#[cfg(test)]
#[path = "content_tests.rs"]
mod tests;
