//! The page list on the left.

use gpui::{AnyElement, App, IntoElement, ParentElement, Styled, Window, div, px};

use crate::empty_view::empty_view;
use crate::frame::Frame;
use crate::rail_item::rail_item;

const WIDTH: f32 = 208.0;
const NARROW: f32 = 720.0;

pub(crate) fn rail(frame: &Frame<'_>, (window, cx): (&mut Window, &mut App)) -> impl IntoElement {
    let pages = frame.state.visible_pages();
    let mut items: Vec<AnyElement> = Vec::with_capacity(pages.len());
    for page in pages {
        items.push(rail_item(frame, page, (&mut *window, &mut *cx)).into_any_element());
    }
    let empty = items.is_empty().then(|| empty_view(cx));
    let shown = f32::from(window.viewport_size().width) >= NARROW;
    let rail = div().flex_none();
    let rail = if shown { rail } else { rail.hidden() };
    rail.w(px(WIDTH))
        .h_full()
        .flex()
        .flex_col()
        .gap(px(2.0))
        .p(px(8.0))
        .overflow_hidden()
        .children(items)
        .children(empty)
}

#[cfg(test)]
#[path = "rail_tests.rs"]
mod tests;
